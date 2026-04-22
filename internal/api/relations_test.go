package api

import (
	"os"
	"strings"
	"testing"
)

// TestParseRelationsMarkdown6ColNewFormat verifies the new 6-column format
// with toKind (26.4.23v1+) is parsed correctly and round-trips safely.
func TestParseRelationsMarkdown6ColNewFormat(t *testing.T) {
	content := `| 目标ID | 目标名称 | 类型 | 关系 | 程度 | 说明 |
|--------|--------|------|------|------|------|
| abao | 阿宝 | agent | 平级协作 | 常用 | AI 伴侣 |
| feishu:ou_abc | 张三 | contact | 服务 | 强 | 客户 |
| telegram:999 | Lilian | contact | 家人 | 强 |  |
`
	rows := parseRelationsMarkdown(content)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d: %+v", len(rows), rows)
	}

	// Row 0: agent
	if rows[0].AgentID != "abao" {
		t.Fatalf("row[0].AgentID = %q", rows[0].AgentID)
	}
	if rows[0].ToKind != "agent" {
		t.Fatalf("row[0].ToKind = %q, want agent", rows[0].ToKind)
	}
	if rows[0].RelationType != "平级协作" {
		t.Fatalf("row[0].RelationType = %q", rows[0].RelationType)
	}

	// Row 1: contact with "服务" (new valid type)
	if rows[1].AgentID != "feishu:ou_abc" {
		t.Fatalf("row[1].AgentID = %q", rows[1].AgentID)
	}
	if rows[1].ToKind != "contact" {
		t.Fatalf("row[1].ToKind = %q, want contact", rows[1].ToKind)
	}
	if rows[1].RelationType != "服务" {
		t.Fatalf("row[1].RelationType = %q", rows[1].RelationType)
	}

	// Row 2: contact with "家人"
	if rows[2].ToKind != "contact" || rows[2].RelationType != "家人" {
		t.Fatalf("row[2] mismatch: %+v", rows[2])
	}
}

// TestParseRelationsMarkdown5ColLegacy verifies legacy 5-column rows still
// parse, with toKind defaulting to "agent".
func TestParseRelationsMarkdown5ColLegacy(t *testing.T) {
	content := `| 成员ID | 成员名称 | 关系类型 | 关系程度 | 说明 |
|--------|--------|--------|--------|------|
| abao | 阿宝 | 平级协作 | 常用 | AI 伴侣 |
| boss | 老板 | 上级 | 强 | 报告线 |
`
	rows := parseRelationsMarkdown(content)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	for i, r := range rows {
		if r.ToKind != "agent" {
			t.Fatalf("row[%d].ToKind = %q, legacy should default to agent", i, r.ToKind)
		}
	}
	if rows[0].RelationType != "平级协作" || rows[1].RelationType != "上级" {
		t.Fatalf("relation type parse wrong: %+v", rows)
	}
}

// TestWriteRelationsFileRoundTrip verifies writing then re-parsing preserves
// all fields including toKind.
func TestWriteRelationsFileRoundTrip(t *testing.T) {
	original := []RelationRow{
		{AgentID: "abao", AgentName: "阿宝", ToKind: "agent", RelationType: "平级协作", Strength: "常用", Desc: "搭档"},
		{AgentID: "feishu:ou_x", AgentName: "老板", ToKind: "contact", RelationType: "服务", Strength: "强", Desc: ""},
	}
	tmp := t.TempDir() + "/RELATIONS.md"
	if err := writeRelationsFile(tmp, original); err != nil {
		t.Fatal(err)
	}
	data, err := readTestFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data, "contact") {
		t.Fatalf("written file missing toKind column:\n%s", data)
	}
	rows := parseRelationsMarkdown(data)
	if len(rows) != 2 {
		t.Fatalf("round-trip row count mismatch: got %d", len(rows))
	}
	if rows[0].ToKind != "agent" || rows[1].ToKind != "contact" {
		t.Fatalf("toKind not preserved: %+v", rows)
	}
	if rows[1].AgentID != "feishu:ou_x" {
		t.Fatalf("contact ID lost: %+v", rows[1])
	}
}

func readTestFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}
