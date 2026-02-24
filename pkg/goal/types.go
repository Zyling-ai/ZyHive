package goal

import "time"

type Status string

const (
	StatusDraft     Status = "draft"
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
	StatusCancelled Status = "cancelled"
)

type GoalType string

const (
	GoalPersonal GoalType = "personal"
	GoalTeam     GoalType = "team"
)

type Milestone struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	DueAt    time.Time `json:"dueAt"`
	Done     bool      `json:"done"`
	AgentIDs []string  `json:"agentIds,omitempty"`
}

type Goal struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	Type        GoalType    `json:"type"`
	AgentIDs    []string    `json:"agentIds"`
	Status      Status      `json:"status"`
	Progress    int         `json:"progress"`
	StartAt     time.Time   `json:"startAt"`
	EndAt       time.Time   `json:"endAt"`
	StartCronID string      `json:"startCronId,omitempty"`
	EndCronID   string      `json:"endCronId,omitempty"`
	Milestones  []Milestone `json:"milestones"`
	Checks      []GoalCheck `json:"checks"`
	CreatedAt   time.Time   `json:"createdAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
}

// GoalCheck 定期检查计划
type GoalCheck struct {
	ID        string    `json:"id"`
	GoalID    string    `json:"goalId"`
	Name      string    `json:"name"`
	Schedule  string    `json:"schedule"`
	TZ        string    `json:"tz,omitempty"`
	AgentID   string    `json:"agentId"`
	Prompt    string    `json:"prompt"`
	CronJobID string    `json:"cronJobId"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
}

// CheckRecord 每次检查的执行记录
type CheckRecord struct {
	ID      string    `json:"id"`
	GoalID  string    `json:"goalId"`
	CheckID string    `json:"checkId"`
	AgentID string    `json:"agentId"`
	RunAt   time.Time `json:"runAt"`
	Output  string    `json:"output"`
	Status  string    `json:"status"`
}
