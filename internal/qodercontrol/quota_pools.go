package qodercontrol

import (
	"strings"
	"time"
)

type DedicatedQuota struct {
	Name      string    `json:"name"`
	Limit     float64   `json:"limit"`
	Used      float64   `json:"used"`
	Remaining float64   `json:"remaining"`
	Available bool      `json:"available"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

type SharedQuota struct {
	Limit     float64 `json:"limit"`
	Used      float64 `json:"used"`
	Remaining float64 `json:"remaining"`
	Available bool    `json:"available"`
	Unlimited bool    `json:"unlimited,omitempty"`
}

type dedicatedQuotaWire struct {
	Name      string  `json:"name"`
	Total     float64 `json:"total"`
	Used      float64 `json:"used"`
	Remaining float64 `json:"remaining"`
	Available *bool   `json:"available"`
	ExpiresAt int64   `json:"expiresAt"`
	Labels    []struct {
		Dimension string            `json:"dimension"`
		Value     string            `json:"value"`
		I18n      map[string]string `json:"valueI18n"`
	} `json:"displayLabels"`
}

type sharedQuotaWire struct {
	Cap       *float64 `json:"cap"`
	Total     *float64 `json:"total"`
	Used      *float64 `json:"used"`
	Remaining *float64 `json:"remaining"`
	Balance   *float64 `json:"balance"`
	Available *bool    `json:"available"`
}

func normalizeSharedQuota(item *sharedQuotaWire) *SharedQuota {
	if item == nil || (item.Cap == nil && item.Total == nil && item.Used == nil && item.Remaining == nil && item.Balance == nil) {
		return nil
	}

	limit, hasLimit := 0.0, false
	if item.Cap != nil {
		limit, hasLimit = *item.Cap, true
	} else if item.Total != nil {
		limit, hasLimit = *item.Total, true
	}
	used := 0.0
	if item.Used != nil {
		used = *item.Used
	}
	remaining, hasRemaining := 0.0, false
	if item.Remaining != nil {
		remaining, hasRemaining = *item.Remaining, true
	} else if item.Balance != nil {
		remaining, hasRemaining = *item.Balance, true
	}
	if !hasLimit && hasRemaining {
		limit, hasLimit = used+remaining, true
	}
	unlimited := hasLimit && limit < 0
	if unlimited {
		remaining = 0
	} else if !hasRemaining && hasLimit {
		remaining = limit - used
		if remaining < 0 {
			remaining = 0
		}
	}
	available := item.Available == nil || *item.Available
	return &SharedQuota{Limit: limit, Used: used, Remaining: remaining, Available: available, Unlimited: unlimited}
}

func normalizeDedicatedQuota(items []dedicatedQuotaWire, now time.Time) []DedicatedQuota {
	var result []DedicatedQuota
	for _, item := range items {
		pack := DedicatedQuota{Name: strings.TrimSpace(item.Name), Limit: item.Total, Used: item.Used, Remaining: item.Remaining,
			Available: item.Available == nil || *item.Available}
		if item.ExpiresAt > 0 {
			pack.ExpiresAt = quotaTime(item.ExpiresAt)
			pack.Available = pack.Available && now.Before(pack.ExpiresAt)
		}
		for _, label := range item.Labels {
			if label.Dimension != "title" {
				continue
			}
			for _, value := range []string{label.I18n["zh-CN"], label.I18n["en-US"], label.Value} {
				if strings.TrimSpace(value) != "" {
					pack.Name = strings.TrimSpace(value)
					break
				}
			}
			break
		}
		if pack.Name == "" {
			pack.Name = "专属积分包"
		}
		result = append(result, pack)
	}
	return result
}
