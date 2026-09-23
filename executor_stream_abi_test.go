package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/bearer"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestExecutorCountTokensMarksEstimate(t *testing.T) {
	service := testExecutorService(&fakeChatTransport{}, nil)
	response, err := service.countTokens(executorRequest(t, false))
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		InputTokens int  `json:"input_tokens"`
		Estimated   bool `json:"estimated"`
	}
	if err := json.Unmarshal(response.Payload, &body); err != nil {
		t.Fatal(err)
	}
	if body.InputTokens == 0 || !body.Estimated {
		t.Fatalf("payload=%s", response.Payload)
	}
}

func TestExecutorABIIdentifierAndUnsupportedHTTP(t *testing.T) {
	identifierEnvelope, err := handleMethod(pluginabi.MethodExecutorIdentifier, nil)
	if err != nil {
		t.Fatal(err)
	}
	identifier := decodeResult[struct {
		Identifier string `json:"identifier"`
	}](t, identifierEnvelope)
	if identifier.Identifier != executorID {
		t.Fatalf("identifier=%q", identifier.Identifier)
	}
	httpEnvelope, err := handleMethod(pluginabi.MethodExecutorHTTPRequest, nil)
	if err != nil {
		t.Fatal(err)
	}
	var decoded testEnvelope[struct{}]
	if err := json.Unmarshal(httpEnvelope, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.OK || decoded.Error == nil || decoded.Error.HTTPStatus != http.StatusNotImplemented {
		t.Fatalf("envelope=%s", httpEnvelope)
	}
}

func TestExecutorStreamEmitFailureCancelsAndCloses(t *testing.T) {
	handle := &fakeChatHandle{chunks: executorChatChunks(), cancelCh: make(chan struct{})}
	closed := make(chan string, 1)
	service := testExecutorService(&fakeChatTransport{handle: handle}, func(method string, payload any) (json.RawMessage, error) {
		switch method {
		case pluginabi.MethodHostStreamEmit:
			return nil, errors.New("downstream closed")
		case pluginabi.MethodHostStreamClose:
			closed <- payload.(map[string]any)["error"].(string)
		}
		return json.RawMessage(`{}`), nil
	})
	if _, err := service.executeStream(context.Background(), executorRequest(t, true)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-handle.cancelCh:
	case <-time.After(time.Second):
		t.Fatal("stream handle was not canceled")
	}
	if message := <-closed; message != "downstream closed" {
		t.Fatalf("close error=%q", message)
	}
}

func TestExecutorStreamEstimatesUsageWhenUpstreamOmitsIt(t *testing.T) {
	handle := &fakeChatHandle{chunks: executorChunksWithoutUsage()}
	var emitted [][]byte
	closed := make(chan struct{})
	service := testExecutorService(&fakeChatTransport{handle: handle}, func(method string, payload any) (json.RawMessage, error) {
		if method == pluginabi.MethodHostStreamEmit {
			emitted = append(emitted, append([]byte(nil), payload.(map[string]any)["payload"].([]byte)...))
		}
		if method == pluginabi.MethodHostStreamClose {
			close(closed)
		}
		return json.RawMessage(`{}`), nil
	})
	if _, err := service.executeStream(context.Background(), executorRequest(t, true)); err != nil {
		t.Fatal(err)
	}
	<-closed
	for _, payload := range emitted {
		var found struct {
			Choices []struct{} `json:"choices"`
			Usage   *struct {
				Estimated bool `json:"estimated"`
			} `json:"usage"`
		}
		if json.Unmarshal(payload, &found) == nil && found.Usage != nil && found.Usage.Estimated {
			if found.Choices == nil {
				t.Fatalf("estimated usage chunk lacks choices: %s", payload)
			}
			return
		}
	}
	t.Fatalf("estimated usage chunk not emitted: %q", emitted)
}

func TestExecutorABIPropagatesUpstream503(t *testing.T) {
	previous := defaultExecutorService
	t.Cleanup(func() { defaultExecutorService = previous })
	defaultExecutorService = testExecutorService(
		&fakeChatTransport{err: &bearer.HTTPError{StatusCode: http.StatusServiceUnavailable}}, nil,
	)
	raw, _ := json.Marshal(executorRequest(t, false))
	envelope, err := handleMethod(pluginabi.MethodExecutorExecute, raw)
	if err != nil {
		t.Fatal(err)
	}
	var decoded testEnvelope[pluginapi.ExecutorResponse]
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.OK || decoded.Error == nil || decoded.Error.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("envelope=%s", envelope)
	}
}

func TestExecutorABIStreamResponseIsJSONSafe(t *testing.T) {
	previous := defaultExecutorService
	t.Cleanup(func() { defaultExecutorService = previous })
	closed := make(chan struct{})
	defaultExecutorService = testExecutorService(
		&fakeChatTransport{handle: &fakeChatHandle{chunks: executorChatChunks()}},
		func(method string, _ any) (json.RawMessage, error) {
			if method == pluginabi.MethodHostStreamClose {
				close(closed)
			}
			return json.RawMessage(`{}`), nil
		},
	)
	raw, _ := json.Marshal(executorRequest(t, true))
	envelope, err := handleMethod(pluginabi.MethodExecutorExecuteStream, raw)
	if err != nil {
		t.Fatal(err)
	}
	var decoded testEnvelope[struct {
		Headers http.Header `json:"headers"`
	}]
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.OK {
		t.Fatalf("envelope=%s", envelope)
	}
	var result struct {
		Headers http.Header `json:"headers"`
	}
	if err := json.Unmarshal(decoded.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Headers.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("headers=%v", result.Headers)
	}
	<-closed
}
