package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestHostHTTPClientRoundTrip(t *testing.T) {
	previous := hostJSONCall
	t.Cleanup(func() { hostJSONCall = previous })
	hostJSONCall = func(method string, payload any) (json.RawMessage, error) {
		if method != pluginabi.MethodHostHTTPDo {
			t.Fatalf("method=%q", method)
		}
		request := payload.(map[string]any)
		if request["host_callback_id"] != "cb-1" {
			t.Fatalf("callback=%v", request["host_callback_id"])
		}
		httpReq := request["request"].(pluginapi.HTTPRequest)
		if httpReq.Method != http.MethodPost || httpReq.URL != "https://qoder.test/token" {
			t.Fatalf("request=%#v", httpReq)
		}
		if string(httpReq.Body) != "hello" || httpReq.Headers.Get("X-Test") != "yes" {
			t.Fatalf("body/headers=%q %#v", httpReq.Body, httpReq.Headers)
		}
		return json.Marshal(pluginapi.HTTPResponse{
			StatusCode: http.StatusCreated,
			Headers:    http.Header{"X-Upstream": {"ok"}},
			Body:       []byte("world"),
		})
	}
	client, err := newHostHTTPClient("cb-1")
	if err != nil {
		t.Fatalf("newHostHTTPClient: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://qoder.test/token", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Test", "yes")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated || resp.Header.Get("X-Upstream") != "ok" || string(body) != "world" {
		t.Fatalf("response=%d %#v %q", resp.StatusCode, resp.Header, body)
	}
}

func TestHostHTTPResponseAcceptsSnakeCase(t *testing.T) {
	var response hostHTTPResponse
	if err := json.Unmarshal([]byte(`{"status_code":202,"headers":{"X-Test":["ok"]},"body":"aGk="}`), &response); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 202 || response.Headers.Get("X-Test") != "ok" || string(response.Body) != "hi" {
		t.Fatalf("response=%#v", response)
	}
}
