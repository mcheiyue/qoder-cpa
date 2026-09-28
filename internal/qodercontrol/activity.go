package qodercontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

const (
	creditsSummaryPath = "/sash/api/v1/ai-conversations/credits-summary"
	seatActivityPath   = "/sash/api/v1/ai-conversations/seat-activity"
	creditsHeatmapPath = "/sash/api/v1/ai-conversations/credits-heatmap"
)

// CreditsSummary is the account credits aggregate.
type CreditsSummary struct {
	TotalCredits float64   `json:"totalCredits"`
	PeakCredits  float64   `json:"peakCredits"`
	SyncedAt     time.Time `json:"synced_at"`
}

// SeatActivity is the account activity/streak snapshot.
type SeatActivity struct {
	CumulativeActiveDays   int       `json:"cumulativeActiveDays"`
	CurrentConsecutiveDays int       `json:"currentConsecutiveDays"`
	MaxConsecutiveDays     int       `json:"maxConsecutiveDays"`
	LastActiveDate         string    `json:"lastActiveDate"`
	SyncedAt               time.Time `json:"synced_at"`
}

// HeatmapItem is one day of credits usage.
type HeatmapItem struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// CreditsHeatmap is the daily credits usage series.
type CreditsHeatmap struct {
	Items    []HeatmapItem `json:"items"`
	Total    float64       `json:"total"`
	SyncedAt time.Time     `json:"synced_at"`
}

// FetchCreditsSummary queries the credits aggregate.
func (c *Client) FetchCreditsSummary(ctx context.Context, cred qoderauth.Credential) (*CreditsSummary, error) {
	body, err := c.doControlRequest(ctx, http.MethodGet, c.buildURL(creditsSummaryPath), cred.AccessToken)
	if err != nil {
		return nil, err
	}
	var payload struct {
		TotalCredits float64 `json:"totalCredits"`
		PeakCredits  float64 `json:"peakCredits"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: credits summary: %v", ErrMalformedResponse, err)
	}
	return &CreditsSummary{TotalCredits: payload.TotalCredits, PeakCredits: payload.PeakCredits, SyncedAt: time.Now().UTC()}, nil
}

// FetchSeatActivity queries activity and streak counters.
func (c *Client) FetchSeatActivity(ctx context.Context, cred qoderauth.Credential) (*SeatActivity, error) {
	body, err := c.doControlRequest(ctx, http.MethodGet, c.buildURL(seatActivityPath), cred.AccessToken)
	if err != nil {
		return nil, err
	}
	var payload struct {
		CumulativeActiveDays   int    `json:"cumulativeActiveDays"`
		CurrentConsecutiveDays int    `json:"currentConsecutiveDays"`
		MaxConsecutiveDays     int    `json:"maxConsecutiveDays"`
		LastActiveDate         string `json:"lastActiveDate"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: seat activity: %v", ErrMalformedResponse, err)
	}
	return &SeatActivity{
		CumulativeActiveDays:   payload.CumulativeActiveDays,
		CurrentConsecutiveDays: payload.CurrentConsecutiveDays,
		MaxConsecutiveDays:     payload.MaxConsecutiveDays,
		LastActiveDate:         payload.LastActiveDate,
		SyncedAt:               time.Now().UTC(),
	}, nil
}

// FetchCreditsHeatmap queries the daily credits usage series for the last days.
func (c *Client) FetchCreditsHeatmap(ctx context.Context, cred qoderauth.Credential, days int) (*CreditsHeatmap, error) {
	endpoint := fmt.Sprintf("%s?days=%d", c.buildURL(creditsHeatmapPath), days)
	body, err := c.doControlRequest(ctx, http.MethodGet, endpoint, cred.AccessToken)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Items []HeatmapItem `json:"items"`
		Total float64       `json:"total"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: credits heatmap: %v", ErrMalformedResponse, err)
	}
	if payload.Items == nil {
		payload.Items = []HeatmapItem{}
	}
	return &CreditsHeatmap{Items: payload.Items, Total: payload.Total, SyncedAt: time.Now().UTC()}, nil
}
