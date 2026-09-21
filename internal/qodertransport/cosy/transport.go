package cosy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrInvalidTestEndpoint is returned when BaseURL is set without AllowTestEndpoint or with a non-loopback address.
var ErrInvalidTestEndpoint = errors.New("cosy: BaseURL requires AllowTestEndpoint=true and loopback address")

// Transport sends COSY-encoded chat requests to a single Qoder endpoint.
type Transport struct {
	httpClient *http.Client
	endpoint   Endpoint
	machineID  string
	userID     string
	orgID      string
	orgTags    []string
	dataPolicy string
	baseURL    string
	timeout    time.Duration
	clock      Clock
}

// Config holds transport configuration.
type Config struct {
	HTTPClient        *http.Client
	Endpoint          Endpoint
	MachineID         string
	UserID            string
	OrganizationID    string
	OrganizationTags  []string
	DataPolicy        string
	BaseURL           string // test-only: override endpoint URL
	Timeout           time.Duration
	AllowTestEndpoint bool  // must be true when BaseURL is set
	Clock             Clock // test-only: override time.Now
}

// NewTransport validates config and returns a Transport.
func NewTransport(cfg Config) (*Transport, error) {
	if cfg.Endpoint != EndpointAPI2 && cfg.Endpoint != EndpointAPI3 {
		return nil, ErrUnknownEndpoint
	}
	if cfg.BaseURL != "" && (!cfg.AllowTestEndpoint || !isLoopbackURL(cfg.BaseURL)) {
		return nil, ErrInvalidTestEndpoint
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Transport{
		httpClient: cfg.HTTPClient,
		endpoint:   cfg.Endpoint,
		baseURL:    cfg.BaseURL,
		timeout:    cfg.Timeout,
		clock:      clock,
		machineID:  cfg.MachineID,
		userID:     cfg.UserID,
		orgID:      cfg.OrganizationID,
		orgTags:    append([]string(nil), cfg.OrganizationTags...),
		dataPolicy: cfg.DataPolicy,
	}, nil
}

// StreamRequest is the input for a streaming chat request.
type StreamRequest struct {
	RuntimeFields RuntimeFields
	RequestBody   []byte // raw JSON from BuildChatBody
	RequestID     string
	CosyVersion   string
	ModelKey      string
	ModelSource   string
}

// StreamResponse holds the SSE parser for a streaming response.
type StreamResponse struct {
	Context context.Context    // internal stream context; use with ParseContext for timeout/cancel
	Cancel  context.CancelFunc // cancels Context and closes response body
	Parser  *SSEParser
	Resp    *http.Response
	state   *streamCloseState
}

type streamCloseState struct {
	mu  sync.Mutex
	err error
}

func (s *streamCloseState) close(body io.Closer) {
	if err := body.Close(); err != nil {
		s.mu.Lock()
		if s.err == nil {
			s.err = err
		}
		s.mu.Unlock()
	}
}

// CloseError reports a response-body close error observed during cancellation.
func (r *StreamResponse) CloseError() error {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	return r.state.err
}

// Stream sends a COSY-encoded request and returns an SSE parser.
func (t *Transport) Stream(ctx context.Context, req StreamRequest) (*StreamResponse, error) {
	if !req.RuntimeFields.Complete() {
		return nil, fmt.Errorf("cosy: runtime fields incomplete")
	}
	parts, err := BuildHTTPRequestAt(t.endpoint, req.RequestBody, req.RuntimeFields, req.RequestID, req.CosyVersion, t.clock())
	if err != nil {
		return nil, fmt.Errorf("cosy: build request: %w", err)
	}
	// Override URL for testing.
	url := parts.URL
	if t.baseURL != "" {
		url = strings.TrimRight(t.baseURL, "/") + "/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1"
	}
	// Internal stream context: respects caller's earlier deadline, applies transport timeout.
	streamCtx, streamCancel := context.WithTimeout(ctx, t.timeout)
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, url, bytes.NewReader(parts.Body))
	if err != nil {
		streamCancel()
		return nil, fmt.Errorf("cosy: create request: %w", err)
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", parts.Auth)
	httpReq.Header.Set("Cache-Control", "no-cache")
	httpReq.Header.Set("Connection", "keep-alive")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Cosy-Business-Product", "cli")
	httpReq.Header.Set("Cosy-Business-Type", "agent")
	httpReq.Header.Set("Cosy-ClientType", "5")
	dataPolicy := t.dataPolicy
	if dataPolicy == "" {
		dataPolicy = "agree"
	}
	httpReq.Header.Set("Cosy-Data-Policy", dataPolicy)
	httpReq.Header.Set("Cosy-Date", parts.Date)
	httpReq.Header.Set("Cosy-Key", parts.CosyKey)
	if t.machineID != "" {
		httpReq.Header.Set("Cosy-MachineId", t.machineID)
		httpReq.Header.Set("Cosy-MachineToken", t.machineID)
		httpReq.Header.Set("Cosy-MachineType", "5")
	}
	if t.userID != "" {
		httpReq.Header.Set("Cosy-User", t.userID)
	}
	if t.orgID != "" {
		httpReq.Header.Set("Cosy-Organization-Id", t.orgID)
	}
	if len(t.orgTags) > 0 {
		httpReq.Header.Set("Cosy-Organization-Tags", strings.Join(t.orgTags, ","))
	}
	if req.ModelKey != "" {
		httpReq.Header.Set("X-Model-Key", req.ModelKey)
		httpReq.Header.Set("X-Model-Source", req.ModelSource)
	}
	httpReq.Header.Set("Cosy-Scene", "assistant")
	httpReq.Header.Set("Cosy-Version", parts.CosyVersion)
	httpReq.Header.Set("Login-Version", "v2")
	httpReq.ContentLength = int64(len(parts.Body))

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		streamCancel()
		if streamCtx.Err() != nil {
			return nil, streamCtx.Err()
		}
		return nil, fmt.Errorf("cosy: HTTP request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		streamCancel()
		body, readErr := readAndClose(resp.Body)
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			category:   classifyHTTPStatus(resp.StatusCode),
			detail:     summarizeErrorBody(body),
			readErr:    readErr,
		}
	}
	// Validate Content-Type is text/event-stream (allow charset).
	ct := resp.Header.Get("Content-Type")
	if !isTextEventStream(ct) {
		streamCancel()
		if err := drainAndClose(resp.Body); err != nil {
			return nil, fmt.Errorf("cosy: close invalid response: %w", err)
		}
		return nil, &InvalidContentTypeError{ContentType: ct}
	}
	state := &streamCloseState{}
	stopClose := context.AfterFunc(streamCtx, func() { state.close(resp.Body) })
	var closeOnce sync.Once
	cancel := func() {
		closeOnce.Do(func() {
			streamCancel()
			state.close(resp.Body)
			stopClose()
		})
	}
	return &StreamResponse{
		Context: streamCtx,
		Cancel:  cancel,
		Parser:  NewSSEParser(resp.Body),
		Resp:    resp,
		state:   state,
	}, nil
}

// isTextEventStream checks whether ct is text/event-stream, allowing optional charset.
func isTextEventStream(ct string) bool {
	ct = strings.TrimSpace(strings.ToLower(ct))
	if !strings.HasPrefix(ct, "text/event-stream") {
		return false
	}
	rest := ct[len("text/event-stream"):]
	return rest == "" || rest[0] == ';' || rest[0] == ' '
}

func readAndClose(body io.ReadCloser) ([]byte, error) {
	data, readErr := io.ReadAll(io.LimitReader(body, 64*1024))
	closeErr := body.Close()
	if readErr != nil && closeErr != nil {
		return data, errors.Join(readErr, closeErr)
	}
	if readErr != nil {
		return data, readErr
	}
	return data, closeErr
}

func drainAndClose(body io.ReadCloser) error {
	_, err := readAndClose(body)
	return err
}
