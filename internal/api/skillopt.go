// SkillOpt (self-evolving skills) REST handlers.
// Routes are mounted under the agents group:
//
//	GET    /api/agents/:id/skills/:skillId/skillopt
//	POST   /api/agents/:id/skills/:skillId/skillopt/predict
//	POST   /api/agents/:id/skills/:skillId/skillopt/oracle
//	GET    /api/agents/:id/skills/:skillId/skillopt/ledger
//	POST   /api/agents/:id/skills/:skillId/skillopt/evolve
//	GET    /api/agents/:id/skills/:skillId/skillopt/proposals
//	POST   /api/agents/:id/skills/:skillId/skillopt/proposals/:pid/accept
//	POST   /api/agents/:id/skills/:skillId/skillopt/proposals/:pid/reject
//	GET    /api/agents/:id/skills/:skillId/skillopt/versions
//	POST   /api/agents/:id/skills/:skillId/skillopt/versions/:ver/rollback
//	POST   /api/agents/:id/skills/:skillId/skillopt/shadow/promote
//	PUT    /api/agents/:id/skills/:skillId/skillopt/config
package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/cron"
	"github.com/Zyling-ai/zyhive/pkg/skillopt"
)

type skilloptHandler struct {
	manager    *agent.Manager
	optMgr     *skillopt.Manager
	cronEngine *cron.Engine
}

func newSkillOptHandler(mgr *agent.Manager, optMgr *skillopt.Manager, cronEngine *cron.Engine) *skilloptHandler {
	return &skilloptHandler{manager: mgr, optMgr: optMgr, cronEngine: cronEngine}
}

// resolve returns the agent's workspace dir and skill id, or writes an error.
func (h *skilloptHandler) resolve(c *gin.Context) (workspaceDir, agentID, skillID string, ok bool) {
	ag, found := h.manager.Get(c.Param("id"))
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return "", "", "", false
	}
	skillID = c.Param("skillId")
	if skillID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "skillId required"})
		return "", "", "", false
	}
	return ag.WorkspaceDir, ag.ID, skillID, true
}

func (h *skilloptHandler) store(c *gin.Context) (*skillopt.Store, string, bool) {
	ws, agentID, skillID, ok := h.resolve(c)
	if !ok {
		return nil, "", false
	}
	return skillopt.NewStore(ws, skillID), agentID, true
}

// Overview GET /skillopt
func (h *skilloptHandler) Overview(c *gin.Context) {
	ws, _, skillID, ok := h.resolve(c)
	if !ok {
		return
	}
	ov, err := h.optMgr.GetOverview(ws, skillID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ov)
}

type predictRequest struct {
	Prediction    string `json:"prediction"`
	ContextDigest string `json:"contextDigest"`
	SessionRef    string `json:"sessionRef"`
}

// Predict POST /skillopt/predict
func (h *skilloptHandler) Predict(c *gin.Context) {
	s, _, ok := h.store(c)
	if !ok {
		return
	}
	var req predictRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Prediction == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prediction required"})
		return
	}
	if err := s.Init(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	entry, err := s.Append(skillopt.LedgerEntry{
		Prediction:    req.Prediction,
		ContextDigest: req.ContextDigest,
		SessionRef:    req.SessionRef,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, entry)
}

type oracleRequest struct {
	EntryID string `json:"entryId"`
	Result  string `json:"result"`
	Hit     *bool  `json:"hit"`
}

// Oracle POST /skillopt/oracle
func (h *skilloptHandler) Oracle(c *gin.Context) {
	s, _, ok := h.store(c)
	if !ok {
		return
	}
	var req oracleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.EntryID == "" || req.Hit == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "entryId and hit required"})
		return
	}
	if err := s.Oracle(req.EntryID, req.Result, *req.Hit); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Ledger GET /skillopt/ledger?limit=100
func (h *skilloptHandler) Ledger(c *gin.Context) {
	s, _, ok := h.store(c)
	if !ok {
		return
	}
	limit := 100
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	entries, err := s.Query(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	baseRate, baseN := s.HitRate(false)
	shadowRate, shadowN := s.HitRate(true)
	c.JSON(http.StatusOK, gin.H{
		"entries":         entries,
		"hitRateBaseline": baseRate,
		"baselineSamples": baseN,
		"hitRateShadow":   shadowRate,
		"shadowSamples":   shadowN,
	})
}

// Evolve POST /skillopt/evolve — force an evolve attempt (runs the LLM).
func (h *skilloptHandler) Evolve(c *gin.Context) {
	ws, agentID, skillID, ok := h.resolve(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	prop, err := h.optMgr.Evolve(ctx, agentID, ws, skillID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if prop == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "proposal": nil, "message": "暂无可进化的失败样本或与历史被拒提案重复"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "proposal": prop})
}

// Proposals GET /skillopt/proposals
func (h *skilloptHandler) Proposals(c *gin.Context) {
	s, _, ok := h.store(c)
	if !ok {
		return
	}
	props, err := s.ListProposals()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, props)
}

// AcceptProposal POST /skillopt/proposals/:pid/accept
func (h *skilloptHandler) AcceptProposal(c *gin.Context) {
	s, _, ok := h.store(c)
	if !ok {
		return
	}
	if err := skillopt.AcceptProposal(s, c.Param("pid")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// RejectProposal POST /skillopt/proposals/:pid/reject
func (h *skilloptHandler) RejectProposal(c *gin.Context) {
	s, _, ok := h.store(c)
	if !ok {
		return
	}
	p, err := s.ReadProposal(c.Param("pid"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if err := skillopt.Reject(s, p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Versions GET /skillopt/versions
func (h *skilloptHandler) Versions(c *gin.Context) {
	s, _, ok := h.store(c)
	if !ok {
		return
	}
	versions, err := s.ListVersions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

// Rollback POST /skillopt/versions/:ver/rollback
func (h *skilloptHandler) Rollback(c *gin.Context) {
	s, _, ok := h.store(c)
	if !ok {
		return
	}
	ver, err := strconv.Atoi(c.Param("ver"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version"})
		return
	}
	if err := skillopt.Rollback(s, ver); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// PromoteShadow POST /skillopt/shadow/promote
func (h *skilloptHandler) PromoteShadow(c *gin.Context) {
	s, _, ok := h.store(c)
	if !ok {
		return
	}
	msg, err := skillopt.Promote(s)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": msg})
}

type skilloptConfigRequest struct {
	Enabled         *bool    `json:"enabled"`
	AutoAccept      *bool    `json:"autoAccept"`
	SampleThreshold *int     `json:"sampleThreshold"`
	PromoteMargin   *float64 `json:"promoteMargin"`
	ShadowMinSample *int     `json:"shadowMinSample"`
	Schedule        string   `json:"schedule"` // 6-field cron expr; default daily 04:00
}

// SetConfig PUT /skillopt/config — toggle evolving + thresholds + maintenance cron.
func (h *skilloptHandler) SetConfig(c *gin.Context) {
	ws, agentID, skillID, ok := h.resolve(c)
	if !ok {
		return
	}
	s := skillopt.NewStore(ws, skillID)

	var req skilloptConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.Init(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ep, err := s.ReadEpoch()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if req.AutoAccept != nil {
		ep.AutoAccept = *req.AutoAccept
	}
	if req.SampleThreshold != nil && *req.SampleThreshold > 0 {
		ep.SampleThreshold = *req.SampleThreshold
	}
	if req.PromoteMargin != nil && *req.PromoteMargin >= 0 {
		ep.PromoteMargin = *req.PromoteMargin
	}
	if req.ShadowMinSample != nil && *req.ShadowMinSample > 0 {
		ep.ShadowMinSample = *req.ShadowMinSample
	}

	// Manage the maintenance cron job on enable/disable.
	enabled := req.Enabled != nil && *req.Enabled
	disabled := req.Enabled != nil && !*req.Enabled

	if (enabled || disabled) && ep.CronJobID != "" && h.cronEngine != nil {
		_ = h.cronEngine.Remove(ep.CronJobID)
		ep.CronJobID = ""
	}
	if enabled && h.cronEngine != nil {
		schedule := req.Schedule
		if schedule == "" {
			schedule = "0 0 4 * * *" // daily 04:00 (6-field, seconds-aware engine)
		}
		job := &cron.Job{
			Name:    "技能进化维护 · " + skillID,
			Remark:  "由 SkillOpt 自动管理，请勿手动删除",
			Enabled: true,
			AgentID: agentID,
			Schedule: cron.Schedule{
				Kind: "cron",
				Expr: schedule,
				TZ:   "Asia/Shanghai",
			},
			Payload: cron.Payload{
				Kind:    "agentTurn",
				Message: skillopt.CronSentinelPrefix + skillID,
			},
			Delivery: cron.Delivery{Mode: "none"},
		}
		if err := h.cronEngine.Add(job); err == nil {
			ep.CronJobID = job.ID
		}
	}

	if err := s.WriteEpoch(ep); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Reflect evolving flag onto the skill meta (best-effort, display-only).
	if req.Enabled != nil {
		h.optMgr.SetSkillEvolving(ws, skillID, *req.Enabled)
	}

	ov, _ := h.optMgr.GetOverview(ws, skillID)
	c.JSON(http.StatusOK, ov)
}
