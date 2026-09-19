package main

import (
	"encoding/json"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
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
		return notImplemented(method)
	case pluginabi.MethodModelStatic, pluginabi.MethodModelForAuth:
		return notImplemented(method)
	case pluginabi.MethodExecutorIdentifier, pluginabi.MethodExecutorExecute,
		pluginabi.MethodExecutorExecuteStream, pluginabi.MethodExecutorCountTokens,
		pluginabi.MethodExecutorHTTPRequest:
		return notImplemented(method)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
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
