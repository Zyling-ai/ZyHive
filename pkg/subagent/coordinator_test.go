package subagent

import (
	"fmt"
	"strings"
	"testing"
)

func TestBuildTaskNotification_Completed(t *testing.T) {
	task := &Task{
		ID:        "task-001",
		Label:     "调研代码库",
		Status:    TaskDone,
		Output:    "发现问题在 src/auth.ts:42",
		StartedAt: 1000,
		EndedAt:   5000,
	}
	n := BuildTaskNotification(task)
	if n.Status != "completed" {
		t.Errorf("status: got %q want %q", n.Status, "completed")
	}
	if n.Usage.DurationMs != 4000 {
		t.Errorf("duration: got %d want 4000", n.Usage.DurationMs)
	}
	xml := n.FormatXML()
	if !strings.Contains(xml, "<task-id>task-001</task-id>") {
		t.Error("XML missing task-id")
	}
	if !strings.Contains(xml, "<status>completed</status>") {
		t.Error("XML missing status")
	}
	if !strings.Contains(xml, "<result>") {
		t.Error("XML missing result when output non-empty")
	}
}

func TestBuildTaskNotification_Failed(t *testing.T) {
	task := &Task{ID: "task-002", Status: TaskError, ErrorMsg: "compile error"}
	n := BuildTaskNotification(task)
	if n.Status != "failed" {
		t.Errorf("status: got %q want %q", n.Status, "failed")
	}
	if !strings.Contains(n.Summary, "compile error") {
		t.Error("summary should contain error message")
	}
}

func TestBuildTaskNotification_Killed(t *testing.T) {
	task := &Task{ID: "task-003", Status: TaskKilled}
	n := BuildTaskNotification(task)
	if n.Status != "killed" {
		t.Errorf("status: got %q want %q", n.Status, "killed")
	}
	if !strings.Contains(n.Summary, "was stopped") {
		t.Error("killed summary should contain 'was stopped'")
	}
}

func TestFormatXML_EmptyOutput(t *testing.T) {
	task := &Task{ID: "empty", Status: TaskDone}
	xml := BuildTaskNotification(task).FormatXML()
	if strings.Contains(xml, "<result></result>") {
		t.Error("empty output should not produce empty <result> tags")
	}
	if !strings.HasPrefix(xml, "<task-notification>") {
		t.Error("XML should start with <task-notification>")
	}
	if !strings.HasSuffix(xml, "</task-notification>") {
		t.Error("XML should end with </task-notification>")
	}
}

func TestFormatXML_EscapesSpecialChars(t *testing.T) {
	task := &Task{
		ID:     "xss",
		Label:  "test <>&",
		Status: TaskDone,
		Output: "<script>alert(1)</script> & 'quote'",
	}
	xml := BuildTaskNotification(task).FormatXML()
	if strings.Contains(xml, "<script>") {
		t.Error("XML should escape < in output")
	}
	if !strings.Contains(xml, "&lt;script&gt;") {
		t.Error("XML should contain escaped &lt;script&gt;")
	}
}

func TestFormatXML_Concurrent(t *testing.T) {
	done := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		go func(n int) {
			task := &Task{ID: fmt.Sprintf("c%d", n), Status: TaskDone, Output: fmt.Sprintf("out%d", n)}
			xml := BuildTaskNotification(task).FormatXML()
			if !strings.HasSuffix(xml, "</task-notification>") {
				t.Errorf("goroutine %d: incomplete XML", n)
			}
			done <- true
		}(i)
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}

func TestCoordinatorSystemPrompt_Contents(t *testing.T) {
	keywords := []string{
		"研究", "综合", "实现", "验证",
		"并行是你的超能力",
		"Continue",
		"综合原则",
		"根据你的发现",
		"Workers 看不到",
	}
	for _, kw := range keywords {
		if !strings.Contains(CoordinatorSystemPrompt, kw) {
			t.Errorf("CoordinatorSystemPrompt missing keyword: %q", kw)
		}
	}
}
