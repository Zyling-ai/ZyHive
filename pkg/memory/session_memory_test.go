package memory

import (
	"strings"
	"testing"
)

func TestShouldExtract_BelowInitThreshold(t *testing.T) {
	cfg := DefaultSessionMemoryConfig
	s := &SessionMemoryState{}
	if s.ShouldExtract(5000, 10, cfg) {
		t.Error("should not extract below init threshold")
	}
}

func TestShouldExtract_InsufficientToolCalls(t *testing.T) {
	cfg := DefaultSessionMemoryConfig
	s := &SessionMemoryState{}
	s.ShouldExtract(15000, 0, cfg) // init
	if s.ShouldExtract(20001, 1, cfg) {
		t.Error("should not extract with < 3 tool calls")
	}
}

func TestShouldExtract_BothThresholdsMet(t *testing.T) {
	cfg := DefaultSessionMemoryConfig
	s := &SessionMemoryState{}
	s.ShouldExtract(15000, 0, cfg) // init
	if !s.ShouldExtract(20001, 3, cfg) {
		t.Error("should extract when both thresholds met")
	}
}

func TestShouldExtract_NoDoubleExtract(t *testing.T) {
	cfg := DefaultSessionMemoryConfig
	s := &SessionMemoryState{}
	s.ShouldExtract(15000, 0, cfg)
	s.ShouldExtract(20001, 3, cfg)
	s.MarkExtracting()
	if s.ShouldExtract(25000, 6, cfg) {
		t.Error("should not extract while already extracting")
	}
}

func TestShouldExtract_AfterDone(t *testing.T) {
	cfg := DefaultSessionMemoryConfig
	s := &SessionMemoryState{}
	s.ShouldExtract(15000, 0, cfg)
	s.ShouldExtract(20001, 3, cfg)
	s.MarkExtracting()
	s.MarkDone(20001)
	// 增长不足
	if s.ShouldExtract(24999, 5, cfg) {
		t.Error("should not extract with insufficient growth after done")
	}
	// 增长足够
	if !s.ShouldExtract(25002, 6, cfg) {
		t.Error("should extract with sufficient growth after done")
	}
}

func TestDefaultTemplate_HasAllSections(t *testing.T) {
	sections := []string{
		"会话标题", "当前状态", "任务规格", "重要文件",
		"工作流程", "错误和修正", "经验教训", "关键结果", "工作日志",
	}
	for _, s := range sections {
		if !strings.Contains(DefaultSessionMemoryTemplate, s) {
			t.Errorf("template missing section: %s", s)
		}
	}
}

func TestBuildExtractionPrompt_ContainsKeyElements(t *testing.T) {
	prompt := BuildExtractionPrompt("notes content", "/path/notes.md")
	checks := map[string]string{
		"notesPath":    "/path/notes.md",
		"notes":        "notes content",
		"criticalRule": "CRITICAL RULES",
		"toolName":     "file_write",
	}
	for name, kw := range checks {
		if !strings.Contains(prompt, kw) {
			t.Errorf("prompt missing %s (%q)", name, kw)
		}
	}
}
