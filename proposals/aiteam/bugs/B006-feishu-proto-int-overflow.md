# B006 · 飞书 protobuf 整数转换溢出

> **严重度**: 🟡 MEDIUM（解析端 DoS，理论可触发，仅在长生命周期 WS 连接中）
> **状态**: 🟡 调研中（决策：是否在 S1 修复或推后）
> **报告人**: aiteam QA pass (S1, 26.5.10v6)
> **工具**: `gosec` G115

---

## 影响

飞书 WebSocket frame 解析器在多处用 `int(varint)` / `int32(varint)` 把 uint64 protobuf varint 强转为有符号小整数，无范围检查。攻击者（或被入侵的飞书租户）可发送恶意 frame：

1. 让 `int(length)` 在 32-bit 平台 wrap 为负数 → 绕过 `if end > len(data)` 边界检查 → 越界读
2. 在 64-bit 平台需要 length ≥ 2^63（9 PB），不现实，但仍是脆弱代码模式
3. service/method 字段类型 int32 → 上游消费这些字段时若 cast 回 uint64 用作 map key 可能撞键

ZyHive 部署目标包括 ARM 32-bit（早期生产用 t4g 之外可能也用 ARM v7 嵌入式），故非全是 false positive。

## 漏洞代码

`pkg/channel/feishu_proto.go`：

```go
// line 86, 93 (Service/Method 字段 — int32 转换不带范围检查)
frame.Service = int32(v)   // ⚠️ v 是 uint64
frame.Method  = int32(v)

// line 105, 243, 262 (length — int 转换；32-bit 平台 int=int32)
end := i + int(length)     // ⚠️ length 是 uint64

// line 334 (反向 cast)
return uint64(int32(...))
```

5 处 G115 警告 + 1 处 G115 反向。

## PoC

伪造 feishu WS frame：

```
field 5 (headers): varint length = 0xFFFFFFFFFFFFFFFF (uint64 max)
    → int(length) on 32-bit = -1
    → end = i + (-1) < len(data) → 边界检查 PASS
    → 后续读 data[i:end] panic 或 越界
```

需要：飞书 WS 已建立（已 paired），攻击者能注入 raw bytes（被入侵的飞书租户 / MITM 旁路）。

## 修复

```go
// 在 case 3/4 (int32 字段):
if v > math.MaxInt32 {
    return nil, fmt.Errorf("field %d value out of int32 range", fieldNumber)
}
frame.Service = int32(v)

// 在 case 5 (length): 同一份显式 cap
if length > uint64(math.MaxInt32) || int(length) < 0 {
    return nil, fmt.Errorf("field 5 length too large")
}
end := i + int(length)
```

## 测试用例

`pkg/channel/feishu_proto_test.go` 新增：

- `TestParseFrame_RejectsOverflowLength`（field 5 length = uint64 max → 期望 error）
- `TestParseFrame_RejectsOverflowServiceID`（field 3 value > MaxInt32 → error）
- `TestParseFrame_NormalShortFrameStillParses`（回归保护）

## 兼容性

- 真实飞书 frame 的 length / service / method 永远在合理范围内 → 真实业务流量不受影响
- 拒绝恶意 frame 比 panic 更安全

## 修复优先级

🟡 **延后到 S3 PR-008 提示词注入防御阶段一起修**（同主题：外部输入 hardening）
