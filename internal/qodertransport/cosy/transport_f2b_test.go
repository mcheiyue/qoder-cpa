package cosy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// F2b probe support: header overrides apply only when explicitly set; the
// default wire must stay byte-identical (guarded by TestF0bHeaderFixture).
func TestStreamHeaderOverrides(t *testing.T) {
	for _, tc := range []struct {
		name      string
		overrides map[string]string
		check     func(t *testing.T, h http.Header)
	}{
		{
			name:      "nil keeps production wire",
			overrides: nil,
			check: func(t *testing.T, h http.Header) {
				if h.Get("Cosy-ClientType") != "5" {
					t.Errorf("Cosy-ClientType = %q, want 5", h.Get("Cosy-ClientType"))
				}
				if h.Get("Cosy-Scene") != "assistant" {
					t.Errorf("Cosy-Scene = %q, want assistant", h.Get("Cosy-Scene"))
				}
				if h.Get("Cosy-MachineOS") != "" {
					t.Errorf("Cosy-MachineOS = %q, want absent", h.Get("Cosy-MachineOS"))
				}
			},
		},
		{
			name: "qoderwork profile replaces and adds",
			overrides: map[string]string{
				"Cosy-ClientType":       "6",
				"Cosy-Scene":            "qwork",
				"Cosy-MachineOS":        "x86_64_win32",
				"User-Agent":            "node",
				"Accept-Language":       "zh-CN,zh;q=0.9",
				"Sec-Fetch-Mode":        "cors",
				"Cosy-Business-Product": "qoder_work",
			},
			check: func(t *testing.T, h http.Header) {
				for k, want := range map[string]string{
					"Cosy-ClientType": "6", "Cosy-Scene": "qwork",
					"Cosy-MachineOS": "x86_64_win32", "User-Agent": "node",
					"Accept-Language": "zh-CN,zh;q=0.9", "Sec-Fetch-Mode": "cors",
					"Cosy-Business-Product": "qoder_work",
				} {
					if got := h.Get(k); got != want {
						t.Errorf("%s = %q, want %q", k, got, want)
					}
				}
				// untouched defaults survive
				if h.Get("Cosy-Business-Type") != "agent" {
					t.Errorf("Cosy-Business-Type = %q, want agent", h.Get("Cosy-Business-Type"))
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var seen http.Header
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				seen = r.Header.Clone()
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: [DONE]\n\n")
			})
			ts := httptest.NewServer(mux)
			defer ts.Close()

			tr, err := NewTransport(Config{
				HTTPClient: ts.Client(), Endpoint: EndpointAPI2,
				BaseURL: ts.URL, AllowTestEndpoint: true,
				MachineID: "machine-1", UserID: "user-1", OrganizationID: "org-1",
			})
			if err != nil {
				t.Fatalf("NewTransport: %v", err)
			}
			body, err := BuildChatBody(f0bInput())
			if err != nil {
				t.Fatalf("BuildChatBody: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			resp, err := tr.Stream(ctx, StreamRequest{
				RuntimeFields: deriveTestFields(t), RequestBody: body, RequestID: "req-1",
				CosyVersion: "1.1.34", ModelKey: "qfmodel", ModelSource: "system",
				HeaderOverrides: tc.overrides,
			})
			if err != nil {
				t.Fatalf("Stream: %v", err)
			}
			resp.Cancel()
			if seen == nil {
				t.Fatal("server saw no request")
			}
			tc.check(t, seen)
		})
	}
}
