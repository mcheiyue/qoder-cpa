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
