package agent

import "testing"

func TestTitleLooksWeak(t *testing.T) {
	weak := []string{
		"",
		"(尚未设置)",
		"你好",
		"你好啊",          // <6 runes
		"请问这个问题",  // starts with "请问"
		"Hello there",
		"hi 大家",
		"OK",
		"好的",
		"开始对话",
	}
	strong := []string{
		"ZyStudio 战略规划与架构决策",
		"Claude API 429 调试",
		"迁移到 Postgres 的方案讨论",
		"生产力革命  x  激励设计",       // >= 6 runes, not a weak prefix
	}
	for _, s := range weak {
		if !titleLooksWeak(s) {
			t.Errorf("expected %q to be weak", s)
		}
	}
	for _, s := range strong {
		if titleLooksWeak(s) {
			t.Errorf("expected %q to NOT be weak", s)
		}
	}
}
