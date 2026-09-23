package cosy

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/qoderstream"
)

func safeErrorCategory(msg string) string { return qoderstream.SafeErrorCategory(msg) }
func statusCategory(code int) string      { return qoderstream.StatusCategory(code) }
func isRateLimitText(content string) bool { return qoderstream.IsRateLimitText(content) }

func parseResetTime(raw string) time.Time {
	var holder struct {
		AgentLimitResetTime json.Number `json:"agentLimitResetTime"`
	}
	if err := json.Unmarshal([]byte(raw), &holder); err != nil || holder.AgentLimitResetTime == "" {
		return time.Time{}
	}
	ms, err := holder.AgentLimitResetTime.Int64()
	if err != nil {
		s, err2 := strconv.Unquote(`"` + holder.AgentLimitResetTime.String() + `"`)
		if err2 != nil {
			return time.Time{}
		}
		ms, err = strconv.ParseInt(s, 10, 64)
		if err != nil {
			return time.Time{}
		}
	}
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}
