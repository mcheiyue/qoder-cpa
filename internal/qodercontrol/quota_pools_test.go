package qodercontrol

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

func TestFetchQuotaAccountsForResourcePools(t *testing.T) {
	for _, tc := range []struct {
		name, usage string
		exhausted   bool
	}{
		{"addon available", `{"isQuotaExceeded":true,"userQuota":{"total":300,"used":300,"remaining":0},"addOnQuota":{"total":100,"used":20,"remaining":80}}`, false},
		{"all exhausted", `{"userQuota":{"total":300,"used":300,"remaining":0},"addOnQuota":{"total":100,"used":100,"remaining":0}}`, true},
		{"unknown", `{"isQuotaExceeded":true}`, false},
		{"dedicated available", `{"userQuota":{"total":300,"used":300,"remaining":0},"dedicatedResourcePackages":[{"total":2000,"used":39,"remaining":1961,"expiresAt":253402214400000,"available":true,"displayLabels":[{"dimension":"title","valueI18n":{"zh-CN":"SOTA 专属积分"}}]}]}`, false},
		{"dedicated expired", `{"userQuota":{"total":300,"used":300,"remaining":0},"dedicatedResourcePackages":[{"total":2000,"remaining":1961,"expiresAt":1000,"available":true}]}`, true},
		{"dedicated unavailable", `{"userQuota":{"total":300,"used":300,"remaining":0},"dedicatedResourcePackages":[{"total":2000,"remaining":1961,"available":false}]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := `{}`
				if r.URL.Path == quotaUsagePath {
					body = tc.usage
				}
				if _, err := io.WriteString(w, body); err != nil {
					t.Error(err)
				}
			}))
			defer srv.Close()
			client, _ := newTestClient(t, srv)
			q, err := client.FetchQuota(context.Background(), qoderauth.Credential{AccessToken: "test"})
			if err != nil {
				t.Fatal(err)
			}
			if q.Exhausted != tc.exhausted {
				t.Fatalf("exhausted=%v want=%v", q.Exhausted, tc.exhausted)
			}
			if tc.name == "dedicated available" {
				if len(q.Dedicated) != 1 {
					t.Fatalf("dedicated=%+v", q.Dedicated)
				}
				p := q.Dedicated[0]
				if p.Name != "SOTA 专属积分" || p.Limit != 2000 || p.Used != 39 || p.Remaining != 1961 || !p.Available || p.ExpiresAt.IsZero() {
					t.Fatalf("pack=%+v", p)
				}
				if q.Remaining != 0 || q.Limit != 300 {
					t.Fatalf("dedicated must not enter general pool: %+v", q)
				}
			}
		})
	}
}
