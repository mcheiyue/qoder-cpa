package main

import (
	"context"
	"encoding/json"

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
		return notImplemented(method)
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

func managementRegister() map[string]any {
	return map[string]any{
		"Routes":    []map[string]string{},
		"Resources": []map[string]string{},
	}
}

func managementHandle(raw []byte) ([]byte, error) {
	return errorEnvelope("not_implemented", "management.handle not yet implemented"), nil
}

// callHostJSON is a placeholder for host RPC — will be wired in later todos.
var hostJSONCall func(method string, payload any) (json.RawMessage, error)

func callHostJSON(method string, payload any) (json.RawMessage, error) {
	if hostJSONCall != nil {
		return hostJSONCall(method, payload)
	}
	return nil, simpleErr("host not connected")
}

type simpleErr string

func (e simpleErr) Error() string { return string(e) }
