// pkg/memory/mmr.go — Maximal Marginal Relevance re-ranking + temporal decay.
//
// MMR improves memory search diversity by penalizing results that are too
// similar to already-selected results, while still preferring results that
// are relevant to the original query.
//
// Usage:
//
//	candidates := idx.retrieveCandidates(queryVec, query, topK*3)
//	candidates = ApplyTemporalDecay(candidates, 30)
//	results := MMR(queryVec, candidates, 0.7, topK)
package memory

import (
	"math"
	"time"
)

// DefaultMMRLambda is the default trade-off parameter λ for MMR.
// Higher λ (e.g. 0.7) favours relevance; lower λ (e.g. 0.3) favours diversity.
const DefaultMMRLambda = 0.7

// DefaultHalfLifeDays is the default temporal decay half-life in days.
// After this many days the score is halved.
const DefaultHalfLifeDays = 30.0

// MMR selects up to topK results from candidates using Maximal Marginal Relevance.
//
// Score formula:
//
//	mmr(d) = λ * sim(query, d) - (1-λ) * max_{d_s ∈ selected} sim(d, d_s)
//
// where sim is cosine similarity when vectors are available, or score-based
// similarity otherwise.
//
// Parameters:
//   - query:      query embedding vector (may be nil → fall back to score-only mode)
//   - candidates: scored candidate results (already retrieved + decay applied)
//   - lambda:     trade-off λ ∈ [0,1]; use DefaultMMRLambda if unsure
//   - topK:       number of results to return
func MMR(query []float32, candidates []SearchResult, lambda float64, topK int) []SearchResult {
	if len(candidates) == 0 {
		return nil
	}
	if topK <= 0 {
		topK = 5
	}
	if topK > len(candidates) {
		topK = len(candidates)
	}
	if lambda < 0 {
		lambda = 0
	}
	if lambda > 1 {
		lambda = 1
	}

	// Track which candidates have been selected (by index)
	selected := make([]SearchResult, 0, topK)
	remaining := make([]int, len(candidates))
	for i := range remaining {
		remaining[i] = i
	}

	// Normalise scores to [0,1] for consistent MMR computation when
	// no query vector is available.
	maxScore := 0.0
	for _, c := range candidates {
		if c.Score > maxScore {
			maxScore = c.Score
		}
	}
	if maxScore == 0 {
		maxScore = 1
	}

	for len(selected) < topK && len(remaining) > 0 {
		bestIdx := -1
		bestMMR := math.Inf(-1)

		for _, ri := range remaining {
			c := candidates[ri]

			// Relevance: cosine(query, d) if we have vectors; else normalised score
			var relevance float64
			if query != nil && len(c.Vec) > 0 {
				relevance = cosineSim(query, c.Vec)
			} else {
				relevance = c.Score / maxScore
			}

			// Diversity penalty: max cosine similarity to already-selected items
			maxSim := 0.0
			for _, s := range selected {
				var sim float64
				if len(c.Vec) > 0 && len(s.Vec) > 0 {
					sim = cosineSim(c.Vec, s.Vec)
				} else {
					// Approximate similarity by score proximity
					diff := math.Abs(c.Score-s.Score) / maxScore
					sim = 1 - diff
				}
				if sim > maxSim {
					maxSim = sim
				}
			}

			mmrScore := lambda*relevance - (1-lambda)*maxSim
			if mmrScore > bestMMR {
				bestMMR = mmrScore
				bestIdx = ri
			}
		}

		if bestIdx < 0 {
			break
		}

		// Move bestIdx from remaining to selected
		selected = append(selected, candidates[bestIdx])
		newRemaining := make([]int, 0, len(remaining)-1)
		for _, ri := range remaining {
			if ri != bestIdx {
				newRemaining = append(newRemaining, ri)
			}
		}
		remaining = newRemaining
	}

	return selected
}

// ApplyTemporalDecay adjusts each result's Score by an exponential decay factor
// based on the age of the source document:
//
//	score *= exp(-ln(2) * age_days / halfLifeDays)
//
// This means after halfLifeDays days the score is halved.
// Results whose CreatedAt is zero (unknown time) are left unchanged.
//
// Parameters:
//   - results:      list of SearchResult to adjust (modified in-place and returned)
//   - halfLifeDays: decay half-life in days; use DefaultHalfLifeDays (30) if unsure
func ApplyTemporalDecay(results []SearchResult, halfLifeDays float64) []SearchResult {
	if halfLifeDays <= 0 {
		halfLifeDays = DefaultHalfLifeDays
	}
	now := time.Now()
	for i := range results {
		if results[i].CreatedAt.IsZero() {
			continue // no time information — skip decay
		}
		ageDays := now.Sub(results[i].CreatedAt).Hours() / 24
		if ageDays < 0 {
			ageDays = 0
		}
		// exp(-ln(2) * age / halfLife) = 0.5 when age == halfLife
		decay := math.Exp(-math.Ln2 * ageDays / halfLifeDays)
		results[i].Score *= decay
	}
	return results
}
