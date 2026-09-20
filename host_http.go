package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type hostHTTPResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

func (r *hostHTTPResponse) UnmarshalJSON(data []byte) error {
	var wire struct {
		StatusCodeCamel int         `json:"StatusCode"`
		StatusCodeSnake int         `json:"status_code"`
		Headers         http.Header `json:"Headers"`
		HeadersLower    http.Header `json:"headers"`
		Body            []byte      `json:"Body"`
		BodyLower       []byte      `json:"body"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	r.StatusCode = wire.StatusCodeCamel
	if r.StatusCode == 0 {
		r.StatusCode = wire.StatusCodeSnake
	}
	r.Headers = wire.Headers
	if r.Headers == nil {
		r.Headers = wire.HeadersLower
	}
	r.Body = wire.Body
	if r.Body == nil {
		r.Body = wire.BodyLower
	}
	return nil
}

type hostRoundTripper struct {
	callbackID string
	call       func(string, any) (json.RawMessage, error)
}

type hostHTTPStreamResponse struct {
	StatusCode int                         `json:"status_code"`
	Headers    http.Header                 `json:"headers"`
	StreamID   string                      `json:"stream_id"`
	Chunks     []pluginapi.HTTPStreamChunk `json:"chunks"`
}

type hostHTTPStreamRead struct {
	Payload []byte `json:"payload"`
	Error   string `json:"error"`
	Done    bool   `json:"done"`
}

type hostStreamBody struct {
	callbackID string
	streamID   string
	call       func(string, any) (json.RawMessage, error)
	initial    []pluginapi.HTTPStreamChunk
	index      int
	reader     *bytes.Reader
	done       bool
	closed     atomic.Bool
	closeOnce  sync.Once
}

func (t hostRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("read host HTTP request body: %w", err)
		}
	}
	request := map[string]any{
		"host_callback_id": t.callbackID,
		"request": pluginapi.HTTPRequest{
			Method: req.Method, URL: req.URL.String(), Headers: req.Header.Clone(), Body: body,
		},
	}
	if strings.Contains(strings.ToLower(req.Header.Get("Accept")), "text/event-stream") {
		return t.roundTripStream(req, request)
	}
	raw, err := t.call(pluginabi.MethodHostHTTPDo, request)
	if err != nil {
		return nil, fmt.Errorf("host HTTP call: %w", err)
	}
	var result hostHTTPResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode host HTTP response: %w", err)
	}
	if result.StatusCode == 0 {
		return nil, fmt.Errorf("host HTTP response missing status code")
	}
	return &http.Response{
		StatusCode: result.StatusCode,
		Header:     result.Headers,
		Body:       io.NopCloser(bytes.NewReader(result.Body)),
		Request:    req,
	}, nil
}

func (t hostRoundTripper) roundTripStream(req *http.Request, request map[string]any) (*http.Response, error) {
	raw, err := t.call(pluginabi.MethodHostHTTPDoStream, request)
	if err != nil {
		return nil, fmt.Errorf("host HTTP stream call: %w", err)
	}
	var result hostHTTPStreamResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode host HTTP stream response: %w", err)
	}
	if result.StatusCode == 0 {
		return nil, fmt.Errorf("host HTTP stream response missing status code")
	}
	if result.StreamID == "" && len(result.Chunks) == 0 {
		return nil, fmt.Errorf("host HTTP stream response missing stream ID")
	}
	return &http.Response{
		StatusCode: result.StatusCode,
		Header:     result.Headers,
		Body: &hostStreamBody{
			callbackID: t.callbackID, streamID: result.StreamID, call: t.call, initial: result.Chunks,
		},
		Request: req,
	}, nil
}

func (b *hostStreamBody) Read(dst []byte) (int, error) {
	if b.closed.Load() {
		return 0, io.EOF
	}
	for {
		if b.reader != nil && b.reader.Len() > 0 {
			return b.reader.Read(dst)
		}
		if b.index < len(b.initial) {
			chunk := b.initial[b.index]
			b.index++
			if chunk.Err != nil {
				return 0, fmt.Errorf("host HTTP stream failed")
			}
			b.reader = bytes.NewReader(chunk.Payload)
			continue
		}
		if b.done || b.streamID == "" {
			return 0, io.EOF
		}
		raw, err := b.call(pluginabi.MethodHostHTTPStreamRead, map[string]any{
			"host_callback_id": b.callbackID, "stream_id": b.streamID,
		})
		if err != nil {
			return 0, fmt.Errorf("host HTTP stream read: %w", err)
		}
		var chunk hostHTTPStreamRead
		if err := json.Unmarshal(raw, &chunk); err != nil {
			return 0, fmt.Errorf("decode host HTTP stream chunk: %w", err)
		}
		if chunk.Error != "" {
			return 0, fmt.Errorf("host HTTP stream failed")
		}
		b.done = chunk.Done
		b.reader = bytes.NewReader(chunk.Payload)
	}
}

func (b *hostStreamBody) Close() error {
	var closeErr error
	b.closeOnce.Do(func() {
		b.closed.Store(true)
		if b.streamID != "" {
			_, closeErr = b.call(pluginabi.MethodHostHTTPStreamClose, map[string]any{
				"host_callback_id": b.callbackID, "stream_id": b.streamID,
			})
		}
	})
	return closeErr
}

func newHostHTTPClient(callbackID string) (*http.Client, error) {
	return newHostHTTPClientWithCall(callbackID, callHostJSON)
}

func newHostHTTPClientWithCall(callbackID string, call func(string, any) (json.RawMessage, error)) (*http.Client, error) {
	if callbackID == "" {
		return nil, fmt.Errorf("host callback ID is required")
	}
	if call == nil {
		return nil, fmt.Errorf("host callback is required")
	}
	return &http.Client{Transport: hostRoundTripper{callbackID: callbackID, call: call}}, nil
}
