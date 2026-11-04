package main

import (
	"encoding/json"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
)

func okEnvelope(result any) ([]byte, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return json.Marshal(pluginabi.Envelope{OK: true, Result: raw})
}

func errorEnvelope(code, message string) []byte {
	return errorEnvelopeStatus(code, message, 0)
}

func errorEnvelopeStatus(code, message string, status int) []byte {
	return errorEnvelopeRetry(code, message, status, 0)
}

// errorEnvelopeRetry attaches the upstream retry hint (seconds) to the failure
// envelope; values <= 0 omit the retry_after_seconds wire field. The host maps
// it to a Retry-After response header (rpc_client.go RetryAfter).
func errorEnvelopeRetry(code, message string, status, retryAfter int) []byte {
	raw, err := json.Marshal(pluginabi.Envelope{
		OK:    false,
		Error: &pluginabi.Error{Code: code, Message: message, HTTPStatus: status, RetryAfterSeconds: retryAfter},
	})
	if err != nil {
		return []byte(`{"ok":false,"error":{"code":"marshal_error","message":"failed to encode error envelope"}}`)
	}
	return raw
}
