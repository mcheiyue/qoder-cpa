package bearer

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestNewTransport_ProductionDefaults(t *testing.T) {
	tr, err := NewTransport(Config{
		Token: "tok_test",
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	if tr == nil {
		t.Fatal("transport is nil")
	}
}

func TestNewTransport_MissingToken(t *testing.T) {
	_, err := NewTransport(Config{})
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}

func TestNewTransport_EmptyToken(t *testing.T) {
	_, err := NewTransport(Config{Token: "  "})
	if err == nil {
		t.Fatal("expected error for blank token")
	}
}

func TestNewTransport_BaseURLRequiresAllowTestEndpoint(t *testing.T) {
	_, err := NewTransport(Config{
		Token:             "tok",
		BaseURL:           "http://localhost:9999",
		AllowTestEndpoint: false,
	})
	if err == nil {
		t.Fatal("expected error: BaseURL without AllowTestEndpoint")
	}
}

func TestNewTransport_BaseURLRejectsNonLoopback(t *testing.T) {
	urls := []string{
		"http://evil.com/steal",
		"https://10.0.0.1:8080",
		"http://192.168.1.1:80",
	}
	for _, url := range urls {
		t.Run(url, func(t *testing.T) {
			_, err := NewTransport(Config{
				Token:             "tok",
				BaseURL:           url,
				AllowTestEndpoint: true,
			})
			if err == nil {
				t.Errorf("expected error for non-loopback BaseURL %q", url)
			}
		})
	}
}

func TestNewTransport_BaseURLRejectsUserinfo(t *testing.T) {
	_, err := NewTransport(Config{
		Token:             "tok",
		BaseURL:           "http://user:pass@127.0.0.1:8080",
		AllowTestEndpoint: true,
	})
	if err == nil {
		t.Fatal("expected error for BaseURL with userinfo")
	}
}

func TestNewTransport_BaseURLRejectsQuery(t *testing.T) {
	_, err := NewTransport(Config{
		Token:             "tok",
		BaseURL:           "http://127.0.0.1:8080?secret=1",
		AllowTestEndpoint: true,
	})
	if err == nil {
		t.Fatal("expected error for BaseURL with query")
	}
}

func TestNewTransport_BaseURLRejectsFragment(t *testing.T) {
	_, err := NewTransport(Config{
		Token:             "tok",
		BaseURL:           "http://127.0.0.1:8080#frag",
		AllowTestEndpoint: true,
	})
	if err == nil {
		t.Fatal("expected error for BaseURL with fragment")
	}
}

func TestNewTransport_AllowTestEndpointLoopbackSucceeds(t *testing.T) {
	tr, err := NewTransport(Config{
		Token:             "tok",
		BaseURL:           "http://127.0.0.1:8080",
		AllowTestEndpoint: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr == nil {
		t.Fatal("transport is nil")
	}
}

func TestNewTransport_AllowTestEndpointHTTPSSucceeds(t *testing.T) {
	tr, err := NewTransport(Config{
		Token:             "tok",
		BaseURL:           "https://localhost:9999",
		AllowTestEndpoint: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr == nil {
		t.Fatal("transport is nil")
	}
}

// requestAssertions captures server-side request details.
type requestAssertions struct {
	mu         sync.Mutex
	calls      int
	method     string
	url        string
	authHeader string
	userAgent  string
	body       []byte
}

func fakeBearerServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *requestAssertions) {
	t.Helper()
	a := &requestAssertions{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.calls++
		a.method = r.Method
		a.url = r.URL.String()
		a.authHeader = r.Header.Get("Authorization")
		a.userAgent = r.Header.Get("User-Agent")
		a.mu.Unlock()
		if handler != nil {
			handler(w, r)
		}
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, a
}

func defaultStreamResponse() string {
	return "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hi\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"gpt-4\",\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\n" +
		"data: [DONE]\n\n"
}
