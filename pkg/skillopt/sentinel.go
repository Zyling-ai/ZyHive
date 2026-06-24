package skillopt

import "strings"

// CronSentinelPrefix marks a cron job payload as a SkillOpt maintenance trigger.
// The skill id follows the prefix, e.g. "__SKILLOPT_MAINTAIN__:my-skill".
//
// This sentinel is intercepted in cmd/aipanel/main.go's cronRunFunc (NOT via
// pool.Run), because cron jobs execute through the subagent runner which does
// not see pool.Run's sentinel handling.
const CronSentinelPrefix = "__SKILLOPT_MAINTAIN__:"

// ParseCronSentinel reports whether message is a SkillOpt maintenance trigger
// and, if so, returns the target skill id ("" means "all skills for the agent").
func ParseCronSentinel(message string) (skillID string, ok bool) {
	if !strings.HasPrefix(message, CronSentinelPrefix) {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(message, CronSentinelPrefix)), true
}
