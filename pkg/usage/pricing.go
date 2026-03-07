// pkg/usage/pricing.go — per-model cost estimation (USD per 1M tokens).
package usage

import "strings"

// EstimateCost returns estimated cost in USD for the given token counts.
func EstimateCost(model string, inputTokens, outputTokens int) float64 {
	in, out := lookupPrice(model)
	return float64(inputTokens)/1_000_000*in + float64(outputTokens)/1_000_000*out
}

// lookupPrice returns [inputPricePerMillion, outputPricePerMillion] in USD.
func lookupPrice(model string) (float64, float64) {
	id := strings.ToLower(model)
	// Anthropic
	switch {
	case contains(id, "claude-opus-4"):        return 15.0, 75.0
	case contains(id, "claude-sonnet-4"):      return 3.0, 15.0
	case contains(id, "claude-haiku-4"):       return 0.8, 4.0
	case contains(id, "claude-3-5-sonnet"):    return 3.0, 15.0
	case contains(id, "claude-3-5-haiku"):     return 0.8, 4.0
	case contains(id, "claude-3-opus"):        return 15.0, 75.0
	case contains(id, "claude-3-sonnet"):      return 3.0, 15.0
	case contains(id, "claude-3-haiku"):       return 0.25, 1.25
	// OpenAI
	case contains(id, "o3-mini"):              return 1.1, 4.4
	case contains(id, "o3"):                   return 10.0, 40.0
	case contains(id, "o1-mini"):              return 3.0, 12.0
	case contains(id, "o1"):                   return 15.0, 60.0
	case contains(id, "gpt-4o-mini"):          return 0.15, 0.6
	case contains(id, "gpt-4o"):               return 2.5, 10.0
	case contains(id, "gpt-4-turbo"):          return 10.0, 30.0
	case contains(id, "gpt-4"):                return 30.0, 60.0
	case contains(id, "gpt-3.5-turbo"):        return 0.5, 1.5
	// DeepSeek
	case contains(id, "deepseek-reasoner"):    return 0.14, 2.19
	case contains(id, "deepseek-chat"):        return 0.07, 1.1
	case contains(id, "deepseek-coder"):       return 0.14, 0.28
	// MiniMax
	case contains(id, "abab6.5s"):             return 0.1, 0.1
	case contains(id, "abab5.5s"):             return 0.05, 0.05
	// Moonshot / Kimi
	case contains(id, "moonshot-v1-128k"):     return 0.06, 0.06
	case contains(id, "moonshot-v1-32k"):      return 0.024, 0.024
	case contains(id, "moonshot-v1-8k"):       return 0.012, 0.012
	// Zhipu
	case contains(id, "glm-4-plus"):           return 0.05, 0.05
	case contains(id, "glm-4"):                return 0.1, 0.1
	case contains(id, "glm-3-turbo"):          return 0.005, 0.005
	}
	// Generic fallback: $1/$2 per 1M tokens
	return 1.0, 2.0
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
