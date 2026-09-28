package qodercontrol

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchCreditsSummarySuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/sash/api/v1/ai-conversations/credits-summary" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		assertG1SashHeaders(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalCredits":1200,"peakCredits":3400}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	summary, err := c.FetchCreditsSummary(context.Background(), g1Credential())
	if err != nil {
		t.Fatalf("FetchCreditsSummary: %v", err)
	}
	if summary.TotalCredits != 1200 || summary.PeakCredits != 3400 {
		t.Errorf("summary = %+v", summary)
	}
	if summary.SyncedAt.IsZero() {
		t.Error("SyncedAt is zero")
	}
}

func TestFetchCreditsSummaryUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchCreditsSummary(context.Background(), g1Credential())
	var upstream *UpstreamError
	if !errors.As(err, &upstream) || upstream.StatusCode != http.StatusUnauthorized {
		t.Fatalf("err = %v, want UpstreamError 401", err)
	}
}

func TestFetchSeatActivitySuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/sash/api/v1/ai-conversations/seat-activity" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		assertG1SashHeaders(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cumulativeActiveDays":42,"currentConsecutiveDays":5,"maxConsecutiveDays":30,"lastActiveDate":"2026-09-28"}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	activity, err := c.FetchSeatActivity(context.Background(), g1Credential())
	if err != nil {
		t.Fatalf("FetchSeatActivity: %v", err)
	}
	if activity.CumulativeActiveDays != 42 || activity.CurrentConsecutiveDays != 5 || activity.MaxConsecutiveDays != 30 {
		t.Errorf("activity = %+v", activity)
	}
	if activity.LastActiveDate != "2026-09-28" {
		t.Errorf("LastActiveDate = %q", activity.LastActiveDate)
	}
}

func TestFetchSeatActivityUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchSeatActivity(context.Background(), g1Credential())
	var upstream *UpstreamError
	if !errors.As(err, &upstream) || upstream.StatusCode != http.StatusUnauthorized {
		t.Fatalf("err = %v, want UpstreamError 401", err)
	}
}

func TestFetchCreditsHeatmapSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/sash/api/v1/ai-conversations/credits-heatmap" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("days"); got != "30" {
			t.Errorf("days = %q, want 30", got)
		}
		assertG1SashHeaders(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"date":"2026-09-27","value":10},{"date":"2026-09-28","value":5}],"total":15}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	heatmap, err := c.FetchCreditsHeatmap(context.Background(), g1Credential(), 30)
	if err != nil {
		t.Fatalf("FetchCreditsHeatmap: %v", err)
	}
	if heatmap.Total != 15 {
		t.Errorf("Total = %v, want 15", heatmap.Total)
	}
	if len(heatmap.Items) != 2 || heatmap.Items[0].Date != "2026-09-27" || heatmap.Items[0].Value != 10 {
		t.Errorf("Items = %+v", heatmap.Items)
	}
}

func TestFetchCreditsHeatmapUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchCreditsHeatmap(context.Background(), g1Credential(), 30)
	var upstream *UpstreamError
	if !errors.As(err, &upstream) || upstream.StatusCode != http.StatusUnauthorized {
		t.Fatalf("err = %v, want UpstreamError 401", err)
	}
}
