package skillopt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const evolverSystemPrompt = `你是「技能进化器」。你只能改写技能说明书 SKILL.md 中的两个受控区：
1. 规则区（rules）：技能的策略规则；
2. 教训区（lessons）：从失败中提炼的教训，按重要性置顶。

下面给你：当前规则区、当前教训区、以及最近一批失败复盘出的归因教训。
请在**最小改动**原则下，产出进化后的「规则区」和「教训区」全文。

硬约束：
- 规则区相对原文净增不得超过 %d 行；教训区净增不得超过 %d 行；
- 只增量改进，不要推翻重写；保留仍然有效的原有条目；
- 每条简洁可执行，中文，使用「- 」开头的列表项；
- 教训区把最重要、最新的教训放最前面。

严格只输出一个 JSON 对象，不要解释、不要 markdown 代码块：
{"rules": "进化后的规则区全文", "lessons": "进化后的教训区全文"}`

// Evolve produces a bounded-edit Proposal from a batch of attributions.
//
//	(nil, nil)  → nothing actionable (no lessons, or deduped against rejection buffer)
//	(p,   nil)  → a pending proposal was written to disk
//	(nil, err)  → LLM / parse / bound-check failure (caller should log & skip)
func Evolve(ctx context.Context, s *Store, attrs []Attribution, callLLM CallLLM) (*Proposal, error) {
	if len(attrs) == 0 {
		return nil, nil
	}
	if callLLM == nil {
		return nil, fmt.Errorf("skillopt: evolver needs a CallLLM")
	}

	live, err := s.ReadSkillMD()
	if err != nil {
		return nil, err
	}
	base := ensureRegions(live)

	oldRules, err := regionInner(base, rulesStart, rulesEnd)
	if err != nil {
		return nil, err
	}
	oldLessons, err := regionInner(base, lessonsStart, lessonsEnd)
	if err != nil {
		return nil, err
	}

	var lessonsBuf strings.Builder
	for _, a := range attrs {
		lessonsBuf.WriteString(fmt.Sprintf("- [%s] %s\n", strings.Join(a.Tags, "/"), a.Lesson))
	}

	user := fmt.Sprintf("【当前规则区】\n%s\n\n【当前教训区】\n%s\n\n【本批失败归因教训】\n%s",
		strings.TrimSpace(oldRules), strings.TrimSpace(oldLessons), lessonsBuf.String())
	system := fmt.Sprintf(evolverSystemPrompt, maxRuleLines, maxLessonLines)

	raw, err := callLLM(ctx, system, user)
	if err != nil {
		return nil, fmt.Errorf("skillopt evolver llm: %w", err)
	}

	out, err := parseEvolveOut(raw)
	if err != nil {
		return nil, err
	}
	newRules := strings.TrimRight(out.Rules.String(), "\n")
	newLessons := strings.TrimRight(out.Lessons.String(), "\n")
	if strings.TrimSpace(newRules) == "" && strings.TrimSpace(newLessons) == "" {
		return nil, nil
	}

	// Bound check: net added non-empty lines must stay within limits.
	if delta := lineDelta(oldRules, newRules); delta > maxRuleLines {
		return nil, fmt.Errorf("skillopt evolver: rules grew by %d lines (> %d)", delta, maxRuleLines)
	}
	if delta := lineDelta(oldLessons, newLessons); delta > maxLessonLines {
		return nil, fmt.Errorf("skillopt evolver: lessons grew by %d lines (> %d)", delta, maxLessonLines)
	}

	// Splice only the inner regions; everything outside stays byte-identical.
	evolved, err := replaceRegion(base, rulesStart, rulesEnd, newRules)
	if err != nil {
		return nil, err
	}
	evolved, err = replaceRegion(evolved, lessonsStart, lessonsEnd, newLessons)
	if err != nil {
		return nil, err
	}

	// Safety net: confirm only the controlled regions changed.
	if a, b := mustSkeleton(base), mustSkeleton(evolved); a != b {
		return nil, fmt.Errorf("skillopt evolver: edit touched content outside controlled regions")
	}

	fp := fingerprint(newRules, newLessons)

	ep, err := s.ReadEpoch()
	if err != nil {
		return nil, err
	}
	if ep.IsRejected(fp) {
		return nil, nil // already tried and failed this exact change
	}

	lessons := make([]string, 0, len(attrs))
	for _, a := range attrs {
		lessons = append(lessons, a.Lesson)
	}
	hitBefore, _ := s.HitRate(false)

	p := Proposal{
		ID:            "prop-" + uuid.New().String()[:8],
		CreatedAt:     time.Now().UnixMilli(),
		Status:        StatusPending,
		FromVersion:   ep.BaselineVersion,
		Rationale:     summarizeRationale(attrs),
		Lessons:       lessons,
		DiffSummary:   fmt.Sprintf("rules %+d 行 · lessons %+d 行", lineDelta(oldRules, newRules), lineDelta(oldLessons, newLessons)),
		NewContent:    evolved,
		Fingerprint:   fp,
		HitRateBefore: hitBefore,
	}
	if err := s.WriteProposal(p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ── SKILL.md region helpers ──────────────────────────────────────────────────

// ensureRegions appends empty controlled regions when the SKILL.md lacks them,
// without disturbing the original body (progressive takeover).
func ensureRegions(content string) string {
	out := content
	if !strings.Contains(out, rulesStart) || !strings.Contains(out, rulesEnd) {
		out = strings.TrimRight(out, "\n") + fmt.Sprintf("\n\n## 进化规则（SkillOpt 自动维护）\n%s\n\n%s\n", rulesStart, rulesEnd)
	}
	if !strings.Contains(out, lessonsStart) || !strings.Contains(out, lessonsEnd) {
		out = strings.TrimRight(out, "\n") + fmt.Sprintf("\n\n## 近期教训（SkillOpt 自动维护）\n%s\n%s\n", lessonsStart, lessonsEnd)
	}
	return out
}

// regionInner returns the text strictly between the start and end markers.
func regionInner(content, start, end string) (string, error) {
	si := strings.Index(content, start)
	if si < 0 {
		return "", fmt.Errorf("skillopt: missing marker %s", start)
	}
	innerStart := si + len(start)
	ei := strings.Index(content[innerStart:], end)
	if ei < 0 {
		return "", fmt.Errorf("skillopt: missing marker %s", end)
	}
	return strings.Trim(content[innerStart:innerStart+ei], "\n"), nil
}

// replaceRegion swaps the inner text between markers, normalising newline framing.
func replaceRegion(content, start, end, newInner string) (string, error) {
	si := strings.Index(content, start)
	if si < 0 {
		return "", fmt.Errorf("skillopt: missing marker %s", start)
	}
	innerStart := si + len(start)
	rel := strings.Index(content[innerStart:], end)
	if rel < 0 {
		return "", fmt.Errorf("skillopt: missing marker %s", end)
	}
	ei := innerStart + rel
	before := content[:innerStart]
	after := content[ei:]
	inner := strings.Trim(newInner, "\n")
	return before + "\n" + inner + "\n" + after, nil
}

// mustSkeleton replaces both region inners with a sentinel for a cheap
// "did anything outside change?" comparison. Markers are guaranteed present
// because Evolve always runs on ensureRegions output.
func mustSkeleton(content string) string {
	r, err := replaceRegion(content, rulesStart, rulesEnd, "@@")
	if err != nil {
		return content
	}
	r, err = replaceRegion(r, lessonsStart, lessonsEnd, "@@")
	if err != nil {
		return content
	}
	return r
}

// lineDelta returns (new non-empty lines) − (old non-empty lines).
func lineDelta(oldInner, newInner string) int {
	return countNonEmptyLines(newInner) - countNonEmptyLines(oldInner)
}

func countNonEmptyLines(s string) int {
	n := 0
	for _, ln := range strings.Split(s, "\n") {
		if strings.TrimSpace(ln) != "" {
			n++
		}
	}
	return n
}

func fingerprint(rules, lessons string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(rules) + "\x00" + strings.TrimSpace(lessons)))
	return hex.EncodeToString(sum[:])
}

func summarizeRationale(attrs []Attribution) string {
	tagSet := map[string]bool{}
	for _, a := range attrs {
		for _, t := range a.Tags {
			tagSet[t] = true
		}
	}
	tags := make([]string, 0, len(tagSet))
	for t := range tagSet {
		tags = append(tags, t)
	}
	return fmt.Sprintf("基于 %d 条失败样本，主要归因：%s", len(attrs), strings.Join(tags, "、"))
}

// ── tolerant evolve-output parsing ───────────────────────────────────────────

type evolveOut struct {
	Rules   stringOrList `json:"rules"`
	Lessons stringOrList `json:"lessons"`
}

func parseEvolveOut(raw string) (evolveOut, error) {
	js := extractJSONObject(raw)
	if js == "" {
		return evolveOut{}, fmt.Errorf("skillopt evolver: no JSON object in reply")
	}
	var out evolveOut
	if err := json.Unmarshal([]byte(js), &out); err != nil {
		return evolveOut{}, fmt.Errorf("skillopt evolver: parse output: %w", err)
	}
	return out, nil
}

func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}

// stringOrList accepts either a JSON string or a JSON array of strings and
// normalises to a single newline-joined string (the model occasionally returns
// a list of bullet lines instead of one block).
type stringOrList string

func (sl *stringOrList) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*sl = stringOrList(s)
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*sl = stringOrList(strings.Join(list, "\n"))
		return nil
	}
	return fmt.Errorf("stringOrList: expected string or []string")
}

// String exposes the underlying value (so callers can use .Rules / .Lessons).
func (sl stringOrList) String() string { return string(sl) }
