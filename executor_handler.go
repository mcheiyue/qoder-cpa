package main

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

func handleExecutorMethod(method string, raw []byte) ([]byte, error) {
	if method == pluginabi.MethodExecutorIdentifier {
		return okEnvelope(struct {
			Identifier string `json:"identifier"`
		}{Identifier: executorID})
	}
	if method == pluginabi.MethodExecutorHTTPRequest {
		return errorEnvelopeStatus("not_supported", "qoder executor does not expose raw HTTP", http.StatusNotImplemented), nil
	}
	var request rpcExecutorRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return errorEnvelopeStatus("invalid_request", "invalid executor request", http.StatusBadRequest), nil
	}
	var result any
	var err error
	switch method {
	case pluginabi.MethodExecutorExecute:
		result, err = defaultExecutorService.execute(context.Background(), request)
	case pluginabi.MethodExecutorExecuteStream:
		response, streamErr := defaultExecutorService.executeStream(context.Background(), request)
		err = streamErr
		result = struct {
			Headers http.Header `json:"headers,omitempty"`
		}{Headers: response.Headers}
	case pluginabi.MethodExecutorCountTokens:
		result, err = defaultExecutorService.countTokens(request)
	}
	if err != nil {
		failure := classifyExecutorError(err)
		return errorEnvelopeStatus(failure.code, failure.message, failure.status), nil
	}
	return okEnvelope(result)
}
