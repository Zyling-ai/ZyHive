package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/subagent"
)

type subagentHandler struct {
	mgr      *subagent.Manager
	agentMgr *agent.Manager
}

// List GET /api/tasks
func (h *subagentHandler) List(c *gin.Context) {
	agentID := c.Query("agentId")
	status := c.Query("status")
	sessionID := c.Query("sessionId")
	tasks := h.mgr.List(agentID)

	// Filter by sessionId if requested (for re-attaching after page reload)
	if sessionID != "" {
		filtered := tasks[:0]
		for _, t := range tasks {
			if t.SpawnedBySession == sessionID {
				filtered = append(filtered, t)
			}
		}
		tasks = filtered
	}

	// Filter by status if requested
	if status != "" {
		filtered := tasks[:0]
		for _, t := range tasks {
			if string(t.Status) == status {
				filtered = append(filtered, t)
			}
		}
		tasks = filtered
	}
	if tasks == nil {
		tasks = []*subagent.Task{}
	}
	c.JSON(http.StatusOK, tasks)
}

// Get GET /api/tasks/:id
func (h *subagentHandler) Get(c *gin.Context) {
	id := c.Param("id")
	task, ok := h.mgr.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	c.JSON(http.StatusOK, task)
}

// Kill DELETE /api/tasks/:id
func (h *subagentHandler) Kill(c *gin.Context) {
	id := c.Param("id")
	if err := h.mgr.Kill(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Spawn POST /api/tasks
// Validates relationship-based permissions before spawning.
// Rules:
//   - taskType "task":   spawnedBy must be superior or peer of agentId
//   - taskType "report": spawnedBy must be subordinate or peer of agentId
//   - taskType "system" or spawnedBy empty: always allowed (cron / internal)
func (h *subagentHandler) Spawn(c *gin.Context) {
	var req struct {
		AgentID   string `json:"agentId" binding:"required"`
		Task      string `json:"task" binding:"required"`
		Label     string `json:"label"`
		Model     string `json:"model"`
		SpawnedBy string `json:"spawnedBy"`
		TaskType  string `json:"taskType"` // "task" | "report" | "system"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Validate target agent exists
	if _, ok := h.agentMgr.Get(req.AgentID); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found: " + req.AgentID})
		return
	}

	taskType := subagent.TaskType(req.TaskType)
	if taskType == "" {
		taskType = subagent.TaskTypeTask
	}

	// Permission check: skip for system tasks or when spawnedBy is not set
	relation := ""
	if req.SpawnedBy != "" && taskType != subagent.TaskTypeSystem {
		mode := "task"
		if taskType == subagent.TaskTypeReport {
			mode = "report"
		}
		allowed, rel := h.agentMgr.CanSpawn(req.SpawnedBy, req.AgentID, mode)
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{
				"error": permissionDeniedMsg(req.SpawnedBy, req.AgentID, mode, h.agentMgr),
			})
			return
		}
		relation = rel
	}

	task, err := h.mgr.Spawn(subagent.SpawnOpts{
		AgentID:   req.AgentID,
		Label:     req.Label,
		Task:      req.Task,
		Model:     req.Model,
		SpawnedBy: req.SpawnedBy,
		TaskType:  taskType,
		Relation:  relation,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, task)
}

// EligibleTargets GET /api/tasks/eligible?from={agentId}&mode={task|report}
// Returns list of agents the caller can interact with + their relation type.
func (h *subagentHandler) EligibleTargets(c *gin.Context) {
	fromID := c.Query("from")
	mode := c.Query("mode")
	if fromID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from is required"})
		return
	}
	if mode == "" {
		mode = "task"
	}
	targets := h.agentMgr.EligibleTargets(fromID, mode)
	if targets == nil {
		targets = []agent.EligibleTarget{}
	}
	c.JSON(http.StatusOK, targets)
}

// permissionDeniedMsg returns a user-friendly error message.
func permissionDeniedMsg(from, to, mode string, mgr *agent.Manager) string {
	fromAg, fok := mgr.Get(from)
	toAg, tok := mgr.Get(to)
	fromName, toName := from, to
	if fok {
		fromName = fromAg.Name
	}
	if tok {
		toName = toAg.Name
	}
	if mode == "task" {
		return fromName + " 没有权限向 " + toName + " 派遣任务（需要上下级或平级协作关系）"
	}
	return fromName + " 没有权限向 " + toName + " 汇报（需要上下级或平级协作关系）"
}
