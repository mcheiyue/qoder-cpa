package qodercontrol

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/cosy"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFetchSignedModelsCNProfileUsesGatewayCN(t *testing.T) {
	var gotURL string
	hc := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotURL = req.URL.String()
		body := `{"data":[{"id":"qfmodel","name":"Qwen Flash"}]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	})}
	c, err := NewClient(hc, DefaultConfig())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	cred := qoderauth.Credential{
		AccessToken: "at", UserID: "user-1",
		RuntimeInfo: "runtime-info", RuntimeKey: "runtime-key",
		Profile: qoderauth.TransportProfileCosyCN,
	}
	models, err := c.FetchModels(context.Background(), cred)
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("len(models)=%d, want 1", len(models))
	}
	if !strings.HasPrefix(gotURL, "https://gateway.qoder.com.cn/algo/api/v2/model/list") {
		t.Fatalf("catalog URL=%q, want gateway.qoder.com.cn model/list", gotURL)
	}
	if strings.Contains(gotURL, "api2.qoder.sh") || strings.Contains(gotURL, "api3.qoder.sh") {
		t.Fatalf("CN profile must not fall back to intl endpoints: %q", gotURL)
	}
}

func TestFetchSignedModelsIntlProfileKeepsIntlEndpoint(t *testing.T) {
	var gotURL string
	hc := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotURL = req.URL.String()
		body := `{"data":[{"id":"qfmodel","name":"Qwen Flash"}]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	})}
	c, err := NewClient(hc, DefaultConfig())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	cred := qoderauth.Credential{
		AccessToken: "at", UserID: "user-1",
		RuntimeInfo: "runtime-info", RuntimeKey: "runtime-key",
		Profile: qoderauth.TransportProfileCosyAPI2,
	}
	if _, err := c.FetchModels(context.Background(), cred); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if !strings.HasPrefix(gotURL, "https://api2.qoder.sh/algo/api/v2/model/list") {
		t.Fatalf("catalog URL=%q, want intl api2 model/list", gotURL)
	}
}

// compile-time reference so the cosy import is exercised during red phase too.
var _ = cosy.EndpointAPI2
