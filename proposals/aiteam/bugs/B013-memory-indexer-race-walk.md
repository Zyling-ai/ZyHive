# B013 · `pkg/memory/indexer.go::filepath.Walk` 回调 race

> **严重度**: 🟢 LOW
> **状态**: 📝 已分析（建议：保留观察）
> **报告人**: aiteam QA pass (S1, 26.5.10v6)
> **工具**: `gosec` G122

---

## 影响

`pkg/memory/indexer.go:103` 用 `filepath.Walk` 而非 `filepath.WalkDir`。gosec G122 警告：Walk 的 callback 拿到的是 `os.FileInfo`，是在 root 时 `Stat` 一次的结果；如果文件在 Walk 期间被替换（rename / unlink），后续在 callback 里再 `os.ReadFile(path)` 拿到的可能不是 Stat 时那份。

实际场景：
- 该 Walk 跑在 agent 自己的 `memory/` 目录下，目录所有者就是当前进程
- 唯一可能引发竞态的是 agent 自己在另一个 goroutine 同时写 memory（自我蒸馏 / consolidator）
- 即使发生竞态，最坏结果是 indexer 当次跑漏一个新文件 → 下次扫到 → 自我修复

## 漏洞代码

`pkg/memory/indexer.go` line 103：

```go
err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
    ...
})
```

## 修复（推荐但非紧急）

切到 `filepath.WalkDir`：

```go
err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
    info, err2 := d.Info()
    ...
})
```

`WalkDir` 在每个 entry 都重新拿 `DirEntry`，并明示 callback 自己决定何时 `Stat`，比 Walk 鲁棒。

## 测试用例

`TestIndexer_HandlesConcurrentWrites`：跑 indexer 同时另一 goroutine 创建/删除文件，确保不 panic 且最终 INDEX.md 正确。

## 兼容性

WalkDir 与 Walk 接口微差异（FileInfo 改 DirEntry），需要适配。

## 修复优先级

🟢 **后续闲时统一切到 WalkDir**（也建议全仓库 grep 其他 `filepath.Walk` 调用点统一）。
