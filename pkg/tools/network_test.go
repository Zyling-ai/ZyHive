package tools

import (
	"strings"
	"testing"
)

func TestAppendToSectionExisting(t *testing.T) {
	body := `# 张三

## 事实
- (AI 通过 network_note 工具追加此处)

## 偏好（AI 观察）
-

## 待跟进
-
`
	out, err := appendToSection(body, "事实", "公司 A 法务合伙人")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "- 公司 A 法务合伙人") {
		t.Fatalf("expected new entry, got:\n%s", out)
	}
	if strings.Contains(out, "- (AI 通过 network_note 工具追加此处)") {
		t.Fatalf("placeholder should be stripped, got:\n%s", out)
	}
	// Other sections intact
	if !strings.Contains(out, "## 偏好（AI 观察）") {
		t.Fatalf("other sections lost:\n%s", out)
	}
}

func TestAppendToSectionMissing(t *testing.T) {
	body := `# 王五

## 事实
-
`
	out, err := appendToSection(body, "待跟进", "约下周复盘")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "## 待跟进\n- 约下周复盘") {
		t.Fatalf("new section not appended cleanly:\n%s", out)
	}
}

func TestAppendToSectionSecondEntry(t *testing.T) {
	body := `# 李四

## 事实
- 来自深圳
`
	out, err := appendToSection(body, "事实", "偏好简短")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "- 来自深圳") || !strings.Contains(out, "- 偏好简短") {
		t.Fatalf("second entry not preserved:\n%s", out)
	}
}

func TestAppendToSectionPartialMatch(t *testing.T) {
	// "偏好" should match "偏好（AI 观察）" section header
	body := `# 测试

## 偏好（AI 观察）
-
`
	out, err := appendToSection(body, "偏好", "直接给结论")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "- 直接给结论") {
		t.Fatalf("did not append to partially-matching section:\n%s", out)
	}
	// Must not have created a duplicate "## 偏好" header
	if strings.Count(out, "## 偏好") != 1 {
		t.Fatalf("section duplicated:\n%s", out)
	}
}
