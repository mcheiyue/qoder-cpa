package main

import (
	"embed"
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

//go:embed web/index.html web/style.css web/api.js web/views.js web/app.js
var qoderWebFiles embed.FS

type webAsset struct {
	body        []byte
	contentType string
}

var qoderWebAssets = loadQoderWebAssets()

func loadQoderWebAssets() map[string]webAsset {
	assets := make(map[string]webAsset, 5)
	for name, contentType := range map[string]string{
		"index.html": "text/html; charset=utf-8",
		"style.css":  "text/css; charset=utf-8",
		"api.js":     "text/javascript; charset=utf-8",
		"views.js":   "text/javascript; charset=utf-8",
		"app.js":     "text/javascript; charset=utf-8",
	} {
		body, err := qoderWebFiles.ReadFile("web/" + name)
		if err != nil {
			panic("qoder: embedded web asset missing: " + name + ": " + err.Error())
		}
		assets[name] = webAsset{body: body, contentType: contentType}
	}
	return assets
}

func qoderWebAssetFor(path string) (webAsset, bool) {
	if path == "/index.html" {
		asset, ok := qoderWebAssets["index.html"]
		return asset, ok
	}
	if len(path) < 2 || path[0] != '/' {
		return webAsset{}, false
	}
	asset, ok := qoderWebAssets[path[1:]]
	return asset, ok
}

func qoderWebResponse(asset webAsset) pluginapi.ManagementResponse {
	return pluginapi.ManagementResponse{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Content-Type":  {asset.contentType},
			"Cache-Control": {"no-store"},
		},
		Body: asset.body,
	}
}
