package promptdef

import "regexp"

// Rule is a single detection rule. A rule fires when its compiled
// regular expression matches any of the candidate content. Each rule
// carries a stable ID (for audit logs) and a short rationale (for the
// rare human review path).
//
// IMPORTANT: rules are best-effort signals, not security boundaries.
// promptdef's job is to *wrap* matching content with an unambiguous
// untrusted-content envelope so the consuming LLM is at least *aware*
// that elevated-privilege text is upstream; the rules deliberately err
// on the side of low false-negatives over low false-positives because
// false-positives are merely cosmetic (the same content is still
// delivered, just clearly labelled).
type Rule struct {
	ID      string
	Pattern *regexp.Regexp
	Reason  string
}

// defaultRules is the v0 rule set. It covers four families of common
// prompt-injection attacks observed in the wild:
//
//   1. "Ignore previous instructions" family — the classic jailbreak.
//   2. Role override — "you are now …", "act as …" specifically asking
//      the model to drop its system prompt.
//   3. System override — explicit attempts to feed a new "system:" /
//      "<|im_start|>system" / "[SYSTEM]" marker.
//   4. Exfil request — "send your prompt to …", "reveal your instructions".
//
// We intentionally keep the list short and English/Chinese-bilingual.
// More exotic encodings (base64, leetspeak, unicode tricks) are not
// covered in v0; future PRs can extend `defaultRules` and add an
// optional small-model classifier.
//
// Each pattern is matched case-insensitive (`(?i)` prefix) so casing
// games don't trivially defeat the rule.
var defaultRules = []Rule{
	{
		ID:      "ignore_previous_en",
		Pattern: regexp.MustCompile(`(?i)\b(ignore|disregard|forget)\s+(all|the|your|any)?\s*(previous|prior|earlier|above)\s+(instruction|prompt|message|rule|context)`),
		Reason:  "jailbreak: ignore previous instructions",
	},
	{
		ID:      "ignore_previous_zh",
		// Verb (忘记/忽略/无视/绕过/放弃) + up to 12 chars (allow modifiers
		// like 上面 / 的 / 所有 / 之前) + noun (指令/约束/...). Liberal middle
		// allows real phrases like "忽略上面的所有约束".
		Pattern: regexp.MustCompile(`(?i)(忘记|忽略|无视|绕过|放弃).{0,12}(指令|指示|提示|规则|约束|限制|身份)`),
		Reason:  "jailbreak: 忽略之前指令（中文）",
	},
	{
		ID:      "you_are_now",
		Pattern: regexp.MustCompile(`(?i)\byou\s+are\s+now\b|\bact\s+as\b|\bpretend\s+to\s+be\b|\bfrom\s+now\s+on,?\s+you\b`),
		Reason:  "role override: you are now / act as ...",
	},
	{
		ID:      "system_override",
		Pattern: regexp.MustCompile(`(?i)(<\s*\|?\s*im_start\s*\|?\s*>\s*system|\[\s*SYSTEM\s*\]|^\s*system\s*:|<<SYS>>|<\|system\|>)`),
		Reason:  "system role marker injection",
	},
	{
		ID:      "reveal_prompt",
		Pattern: regexp.MustCompile(`(?i)(reveal|show|print|display|disclose|tell\s+me)\s+(your|the)?\s*(system\s+)?(prompt|instructions?|rules)`),
		Reason:  "system prompt exfiltration",
	},
	{
		ID:      "reveal_prompt_zh",
		Pattern: regexp.MustCompile(`(打印|输出|告诉我|展示|泄露|公开).{0,8}(系统提示|系统指令|prompt|提示词|你的指令)`),
		Reason:  "system prompt 泄露请求（中文）",
	},
	{
		ID:      "developer_mode",
		Pattern: regexp.MustCompile(`(?i)(developer|dev|jailbreak|do\s+anything\s+now|DAN|god)\s+mode`),
		Reason:  "jailbreak persona (DAN / dev mode / god mode)",
	},
	{
		ID:      "exfil_credentials",
		// Allow "API key" (space), "API_key" / "api-key" / "apikey", and
		// permit a bit more slack between the verb and the secret noun.
		Pattern: regexp.MustCompile(`(?i)(send|post|email|leak|upload|exfil)\b.{0,60}\b(token|secret|api[\s_-]?key|password|credential)`),
		Reason:  "credential exfiltration attempt",
	},
	{
		ID:      "indirect_url_inject",
		Pattern: regexp.MustCompile(`(?i)(fetch|curl|wget|read|navigate|browse)\s+(this\s+url|the\s+following\s+url|https?://)`),
		Reason:  "indirect injection via URL",
	},
}

// DefaultRules returns a copy of the v0 rule slice. Callers may extend
// it for tests or per-deployment customisation.
func DefaultRules() []Rule {
	out := make([]Rule, len(defaultRules))
	copy(out, defaultRules)
	return out
}
