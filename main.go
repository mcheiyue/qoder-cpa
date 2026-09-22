package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const abiVersion = pluginabi.ABIVersion

func main() {}

func handleMethod(method string, raw []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		return okEnvelope(registration())
	case pluginabi.MethodManagementRegister:
		return okEnvelope(managementRegister())
	case pluginabi.MethodManagementHandle:
		return managementHandle(raw)
	case pluginabi.MethodAuthIdentifier, pluginabi.MethodAuthParse,
		pluginabi.MethodAuthLoginStart, pluginabi.MethodAuthLoginPoll,
		pluginabi.MethodAuthRefresh:
		return handleAuthMethod(method, raw)
	case pluginabi.MethodModelStatic, pluginabi.MethodModelForAuth:
		return handleModelMethod(method, raw)
	case pluginabi.MethodExecutorIdentifier, pluginabi.MethodExecutorExecute,
		pluginabi.MethodExecutorExecuteStream, pluginabi.MethodExecutorCountTokens,
		pluginabi.MethodExecutorHTTPRequest:
		return handleExecutorMethod(method, raw)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func handleModelMethod(method string, raw []byte) ([]byte, error) {
	var result pluginapi.ModelResponse
	var err error
	if method == pluginabi.MethodModelStatic {
		result = staticModels()
	} else {
		result, err = defaultAuthService.models(context.Background(), raw)
	}
	if err != nil {
		return errorEnvelope("model_error", err.Error()), nil
	}
	return okEnvelope(result)
}

func handleAuthMethod(method string, raw []byte) ([]byte, error) {
	ctx := context.Background()
	var result any
	var err error
	switch method {
	case pluginabi.MethodAuthIdentifier:
		result = struct {
			Identifier string `json:"identifier"`
		}{Identifier: qoderauth.Provider}
	case pluginabi.MethodAuthParse:
		result, err = defaultAuthService.parse(raw)
	case pluginabi.MethodAuthLoginStart:
		result, err = defaultAuthService.start(ctx, raw)
	case pluginabi.MethodAuthLoginPoll:
		result, err = defaultAuthService.poll(ctx, raw)
	case pluginabi.MethodAuthRefresh:
		result, err = defaultAuthService.refresh(ctx, raw)
	}
	if err != nil {
		return errorEnvelope("auth_error", err.Error()), nil
	}
	return okEnvelope(result)
}

func notImplemented(method string) ([]byte, error) {
	return errorEnvelope("not_implemented", method+" not yet implemented"), nil
}

func managementHandle(raw []byte) ([]byte, error) {
	var request pluginapi.ManagementRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return errorEnvelopeStatus("invalid_request", "invalid management request", 400), nil
	}
	path := managementPath(request.Path)
	var kind string
	switch {
	case path == "/qoder/accounts":
		kind = "accounts"
	case path == "/qoder/accounts/profile":
		kind = "profile"
	case path == "/qoder/accounts/quota/refresh":
		kind = "quota-refresh"
	case path == "/index.html" || strings.HasSuffix(path, "/qoder/index.html"):
		kind = "web"
	default:
		return errorEnvelopeStatus("not_found", "management route not found", 404), nil
	}
	response, err := (managementHandler{kind: kind}).HandleManagement(context.Background(), request)
	if err != nil {
		return errorEnvelopeStatus("management_error", err.Error(), 500), nil
	}
	return okEnvelope(response)
}

func managementPath(path string) string {
	path = strings.TrimSpace(path)
	for _, prefix := range []string{"/v0/management", "/v0/resource/plugins/" + pluginID} {
		if strings.HasPrefix(path, prefix+"/") {
			return strings.TrimPrefix(path, prefix)
		}
	}
	return path
}

// callHostJSON is the testable seam backed by the C ABI host callback in cabi.go.
var hostJSONCall func(method string, payload any) (json.RawMessage, error)

func callHostJSON(method string, payload any) (json.RawMessage, error) {
	if hostJSONCall != nil {
		return hostJSONCall(method, payload)
	}
	return nil, simpleErr("host not connected")
}

type simpleErr string

func (e simpleErr) Error() string { return string(e) }
