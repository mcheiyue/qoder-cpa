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

func TestFetchQuotaAccountsForSharedResourcePackage(t *testing.T) {
	for _, tc := range []struct {
		name, usage       string
		limit, used, left float64
		available         bool
		unlimited         bool
		exhausted         bool
	}{
		{"shared only", `{"orgResourcePackage":{"cap":6000,"total":8000,"used":254,"remaining":5746,"available":true}}`, 6000, 254, 5746, true, false, false},
		{"shared rescues exhausted personal quota", `{"isQuotaExceeded":true,"userQuota":{"total":300,"used":300,"remaining":0},"orgResourcePackage":{"cap":6000,"used":254,"remaining":5746,"available":true}}`, 6000, 254, 5746, true, false, false},
		{"unavailable shared does not rescue account", `{"isQuotaExceeded":true,"userQuota":{"total":300,"used":300,"remaining":0},"orgResourcePackage":{"cap":6000,"used":254,"remaining":5746,"available":false}}`, 6000, 254, 5746, false, false, true},
		{"unlimited ignores placeholder remaining", `{"orgResourcePackage":{"cap":-1,"used":254,"remaining":1000000,"available":true}}`, -1, 254, 0, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == quotaUsagePath {
					if _, err := io.WriteString(w, tc.usage); err != nil {
						t.Error(err)
					}
					return
				}
				if _, err := io.WriteString(w, `{}`); err != nil {
					t.Error(err)
				}
			}))
			defer srv.Close()

			client, _ := newTestClient(t, srv)
			quota, err := client.FetchQuota(context.Background(), qoderauth.Credential{AccessToken: "test"})
			if err != nil {
				t.Fatal(err)
			}
			shared := quota.Shared
			if shared == nil || shared.Limit != tc.limit || shared.Used != tc.used || shared.Remaining != tc.left || shared.Available != tc.available || shared.Unlimited != tc.unlimited {
				t.Fatalf("shared=%+v, want limit=%v used=%v remaining=%v available=%v unlimited=%v", shared, tc.limit, tc.used, tc.left, tc.available, tc.unlimited)
			}
			if quota.Exhausted != tc.exhausted {
				t.Fatalf("exhausted=%v, want %v; quota=%+v", quota.Exhausted, tc.exhausted, quota)
			}
		})
	}
}
