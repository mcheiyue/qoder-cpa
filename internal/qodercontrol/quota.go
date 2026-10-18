package qodercontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

// Quota holds account quota/subscription information.
type Quota struct {
	// Nickname/Email 来自 /api/v3/user/status；auth 文件本身不含这两个元数据，
	// 缺了它们管理面板只能显示文件名（qoder-<sha256>.json）。仅用于展示。
	Nickname   string  `json:"nickname,omitempty"`
	Email      string  `json:"email,omitempty"`
	PlanTier   string  `json:"plan_tier,omitempty"`
	UserType   string  `json:"user_type,omitempty"`
	PaidPlan   bool    `json:"paid_plan"`
	Limit      float64 `json:"limit"`
	Used       float64 `json:"used"`
	Remaining  float64 `json:"remaining"`
	AgentLimit float64 `json:"agent_limit,omitempty"`
	// AddOn 是独立于主额度的加成额度（上游 addOnQuota，与 userQuota.total 分开计量）。
	AddOn          float64   `json:"add_on,omitempty"`
	AddOnRemaining float64   `json:"add_on_remaining,omitempty"`
	Exhausted      bool      `json:"exhausted"`
	Unit           string    `json:"unit,omitempty"`
	ResetAt        time.Time `json:"reset_at,omitempty"`
	PeriodStart    time.Time `json:"period_start,omitempty"`
	PeriodEnd      time.Time `json:"period_end,omitempty"`
	UpgradeURL     string    `json:"upgrade_url,omitempty"`
	Error          string    `json:"error,omitempty"`
	SyncedAt       time.Time `json:"synced_at"`
}

type quotaUsageResponse struct {
	UserType        string  `json:"userType"`
	IsQuotaExceeded bool    `json:"isQuotaExceeded"`
	ExpiresAt       int64   `json:"expiresAt"`
	UpgradeURL      string  `json:"upgradeUrl"`
	AgentLimit      float64 `json:"agentLimit"`
	AgentLimitSnake float64 `json:"agent_limit"`
	UserQuota       struct {
		Total     float64 `json:"total"`
		Used      float64 `json:"used"`
		Remaining float64 `json:"remaining"`
		Unit      string  `json:"unit"`
	} `json:"userQuota"`
	AddOnQuota struct {
		Total     float64 `json:"total"`
		Used      float64 `json:"used"`
		Remaining float64 `json:"remaining"`
		Unit      string  `json:"unit"`
	} `json:"addOnQuota"`
}

type quotaPlanResponse struct {
	UserType     string `json:"user_type"`
	PlanTierName string `json:"plan_tier_name"`
	IsPaidPlan   bool   `json:"is_paid_plan"`
	StartDate    int64  `json:"start_date"`
	EndDate      int64  `json:"end_date"`
}

type quotaStatusResponse struct {
	Name            string  `json:"name"`
	Email           string  `json:"email"`
	UserType        string  `json:"userType"`
	UserTag         string  `json:"userTag"`
	Plan            string  `json:"plan"`
	AgentLimit      float64 `json:"agentLimit"`
	AgentLimitSnake float64 `json:"agent_limit"`
	IsQuotaExceeded bool    `json:"isQuotaExceeded"`
	NextResetAt     int64   `json:"nextResetAt"`
}

const (
	quotaUsagePath  = "/api/v2/quota/usage"
	quotaPlanPath   = "/api/v2/user/plan"
	quotaStatusPath = "/api/v3/user/status"
)

// FetchQuota retrieves quota/subscription info from Qoder.
// The three reads are independent. A partial snapshot is useful to the
// management UI, while ErrQuotaUnknown is returned only when none succeeded.
// This method never invalidates credentials.
func (c *Client) FetchQuota(ctx context.Context, cred qoderauth.Credential) (*Quota, error) {
	quota := &Quota{SyncedAt: time.Now().UTC()}
	var firstErr error
	succeeded := false

	usageBody, usageErr := c.doRequest(ctx, http.MethodGet, c.buildURL(quotaUsagePath), cred.AccessToken)
	if usageErr == nil {
		var usage quotaUsageResponse
		if err := json.Unmarshal(usageBody, &usage); err != nil {
			firstErr = fmt.Errorf("usage: %w", err)
		} else {
			succeeded = true
			quota.UserType = strings.TrimSpace(usage.UserType)
			quota.Limit = usage.UserQuota.Total
			quota.Used = usage.UserQuota.Used
			quota.Remaining = usage.UserQuota.Remaining
			quota.Unit = strings.TrimSpace(usage.UserQuota.Unit)
			quota.UpgradeURL = strings.TrimSpace(usage.UpgradeURL)
			quota.Exhausted = usage.IsQuotaExceeded || (quota.Limit > 0 && quota.Remaining <= 0)
			quota.AgentLimit = usage.AgentLimit
			if quota.AgentLimit == 0 {
				quota.AgentLimit = usage.AgentLimitSnake
			}
			if usage.ExpiresAt > 0 {
				quota.PeriodEnd = quotaTime(usage.ExpiresAt)
			}
			quota.AddOn = usage.AddOnQuota.Total
			quota.AddOnRemaining = usage.AddOnQuota.Remaining
		}
	} else {
		firstErr = usageErr
	}

	planBody, planErr := c.doRequest(ctx, http.MethodGet, c.buildURL(quotaPlanPath), cred.AccessToken)
	if planErr == nil {
		var plan quotaPlanResponse
		if err := json.Unmarshal(planBody, &plan); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("plan: %w", err)
			}
		} else {
			succeeded = true
			quota.PlanTier = strings.TrimSpace(plan.PlanTierName)
			if quota.UserType == "" {
				quota.UserType = strings.TrimSpace(plan.UserType)
			}
			quota.PaidPlan = plan.IsPaidPlan
			if plan.StartDate > 0 {
				quota.PeriodStart = quotaTime(plan.StartDate)
			}
			if plan.EndDate > 0 {
				quota.PeriodEnd = quotaTime(plan.EndDate)
			}
		}
	} else if firstErr == nil {
		firstErr = planErr
	}

	statusBody, statusErr := c.doRequest(ctx, http.MethodGet, c.buildURL(quotaStatusPath), cred.AccessToken)
	if statusErr == nil {
		var status quotaStatusResponse
		if err := json.Unmarshal(statusBody, &status); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("status: %w", err)
			}
		} else {
			succeeded = true
			quota.Nickname = strings.TrimSpace(status.Name)
			quota.Email = strings.TrimSpace(status.Email)
			if quota.UserType == "" {
				quota.UserType = strings.TrimSpace(status.UserType)
			}
			if quota.PlanTier == "" {
				quota.PlanTier = strings.TrimSpace(status.UserTag)
				if quota.PlanTier == "" {
					quota.PlanTier = strings.TrimSpace(status.Plan)
				}
			}
			if quota.AgentLimit == 0 {
				quota.AgentLimit = status.AgentLimit
				if quota.AgentLimit == 0 {
					quota.AgentLimit = status.AgentLimitSnake
				}
			}
			if status.IsQuotaExceeded {
				quota.Exhausted = true
			}
			if status.NextResetAt > 0 {
				quota.ResetAt = quotaTime(status.NextResetAt)
			}
		}
	} else if firstErr == nil {
		firstErr = statusErr
	}

	if quota.Unit == "" && (quota.Limit != 0 || quota.Remaining != 0) {
		quota.Unit = "credits"
	}
	if firstErr != nil {
		quota.Error = summarizeQuotaError(firstErr)
	}
	if !succeeded {
		return quota, ErrQuotaUnknown
	}
	return quota, nil
}

func quotaTime(value int64) time.Time {
	if value > 1e11 {
		value /= 1000
	}
	return time.Unix(value, 0).UTC()
}

func summarizeQuotaError(err error) string {
	var upstream *UpstreamError
	if errors.As(err, &upstream) {
		return fmt.Sprintf("upstream HTTP %d", upstream.StatusCode)
	}
	if errors.Is(err, ErrResponseTooLarge) {
		return "response too large"
	}
	if errors.Is(err, ErrMalformedResponse) {
		return "malformed response"
	}
	if errors.Is(err, ErrUpstreamFailed) {
		return "upstream request failed"
	}
	return "quota status unavailable"
}
