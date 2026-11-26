package main

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type webEnvelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func webGet(t *testing.T, method, path string) webEnvelope {
	t.Helper()
	payload, err := json.Marshal(pluginapi.ManagementRequest{Method: method, Path: path})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	raw, err := handleMethod(pluginabi.MethodManagementHandle, payload)
	if err != nil {
		t.Fatalf("handleMethod: %v", err)
	}
	var envelope webEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return envelope
}

func decodeWebResponse(t *testing.T, envelope webEnvelope) pluginapi.ManagementResponse {
	t.Helper()
	if !envelope.OK {
		t.Fatalf("management envelope not ok: %s", envelope.Result)
	}
	var response pluginapi.ManagementResponse
	if err := json.Unmarshal(envelope.Result, &response); err != nil {
		t.Fatalf("decode management response: %v", err)
	}
	return response
}

func TestWebResourcesServeEmbeddedAssets(t *testing.T) {
	wantTypes := map[string]string{
		"index.html": "text/html; charset=utf-8",
		"style.css":  "text/css; charset=utf-8",
		"api.js":     "text/javascript; charset=utf-8",
		"views.js":   "text/javascript; charset=utf-8",
		"app.js":     "text/javascript; charset=utf-8",
	}
	if len(qoderWebAssets) != len(wantTypes) {
		t.Fatalf("embedded assets=%d want %d", len(qoderWebAssets), len(wantTypes))
	}
	for name, contentType := range wantTypes {
		asset, ok := qoderWebAssets[name]
		if !ok || len(asset.body) == 0 {
			t.Fatalf("embedded asset %q missing or empty", name)
		}
		if asset.contentType != contentType {
			t.Fatalf("asset %q content type=%q want %q", name, asset.contentType, contentType)
		}
		for _, path := range []string{"/v0/resource/plugins/qoder/" + name, "/" + name} {
			response := decodeWebResponse(t, webGet(t, http.MethodGet, path))
			if response.StatusCode != http.StatusOK {
				t.Fatalf("path %q status=%d", path, response.StatusCode)
			}
			if got := response.Headers.Get("Content-Type"); got != contentType {
				t.Fatalf("path %q Content-Type=%q want %q", path, got, contentType)
			}
			if got := response.Headers.Get("Cache-Control"); got != "no-store" {
				t.Fatalf("path %q Cache-Control=%q want no-store", path, got)
			}
			if string(response.Body) != string(asset.body) {
				t.Fatalf("path %q body does not match embedded asset", path)
			}
		}
	}
}

func TestWebIndexReferencesOnlyServedLocalAssets(t *testing.T) {
	index := string(qoderWebAssets["index.html"].body)
	references := regexp.MustCompile(`(?:href|src)="([^"]+)"`).FindAllStringSubmatch(index, -1)
	served := 0
	for _, match := range references {
		reference := match[1]
		if strings.HasPrefix(reference, "#") {
			continue
		}
		if strings.Contains(reference, "://") || strings.HasPrefix(reference, "//") {
			t.Fatalf("index must not reference external resource %q", reference)
		}
		name := strings.TrimPrefix(reference, "./")
		if _, ok := qoderWebAssets[name]; !ok {
			t.Fatalf("index references asset %q outside the allowlist", reference)
		}
		served++
	}
	if served < 4 {
		t.Fatalf("index references=%d want stylesheet and three scripts", served)
	}
}

func TestWebResourceRegistrationCoversAllowlist(t *testing.T) {
	resources := qoderWebResourceRegistration()
	if len(resources) != len(qoderWebAssets) {
		t.Fatalf("resources=%d assets=%d", len(resources), len(qoderWebAssets))
	}
	menus := 0
	declared := map[string]bool{}
	for _, resource := range resources {
		name := strings.TrimPrefix(resource["path"], "/")
		if _, ok := qoderWebAssets[name]; !ok {
			t.Fatalf("resource %q is not an embedded asset", resource["path"])
		}
		declared[name] = true
		if resource["menu"] != "" {
			menus++
			if resource["path"] != "/index.html" || resource["menu"] != "Qoder" {
				t.Fatalf("menu must be the Qoder entry: %v", resource)
			}
		}
	}
	if menus != 1 {
		t.Fatalf("menu resources=%d want 1", menus)
	}
	for name := range qoderWebAssets {
		if !declared[name] {
			t.Fatalf("asset %q is not declared", name)
		}
	}
}

func TestWebResourceRejectsUnknownForeignAndNestedPaths(t *testing.T) {
	for _, path := range []string{
		"/missing.js",
		"/v0/resource/plugins/qoder/missing.js",
		"/v0/resource/plugins/qoder/../index.html",
		"/v0/resource/plugins/qoder/nested/index.html",
		"/v0/resource/plugins/qoder/screenshot.png",
		"/v0/resource/plugins/other/index.html",
		"/v0/resource/plugins/qoder-extra/index.html",
	} {
		envelope := webGet(t, http.MethodGet, path)
		if envelope.OK {
			t.Fatalf("path %q unexpectedly served", path)
		}
		if envelope.Error == nil || envelope.Error.Code != "not_found" {
			t.Fatalf("path %q error=%+v want not_found", path, envelope.Error)
		}
	}
}

func TestWebResourceRejectsNonGet(t *testing.T) {
	envelope := webGet(t, http.MethodPost, "/style.css")
	if envelope.OK {
		t.Fatal("static resource must not be served for POST")
	}
	if envelope.Error == nil || envelope.Error.Code != "not_found" {
		t.Fatalf("error=%+v want not_found", envelope.Error)
	}
}
