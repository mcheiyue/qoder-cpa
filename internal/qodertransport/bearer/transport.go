package bearer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultEndpoint = "https://api2-v2.qoder.sh/model/v1/chat/completions"
	defaultUA       = "Qoder-CPA/1.0"
)

// ErrInvalidTestEndpoint is returned when BaseURL is set without AllowTestEndpoint or with a non-loopback address.
var ErrInvalidTestEndpoint = errors.New("bearer: BaseURL requires AllowTestEndpoint=true and loopback address")

// ErrEmptyModel is returned when StreamRequest.Model is empty.
var ErrEmptyModel = errors.New("bearer: model must not be empty")

// ErrEmptyMessages is returned when StreamRequest.Messages is empty.
var ErrEmptyMessages = errors.New("bearer: messages must not be empty")

// Transport sends OpenAI-compatible chat requests to a single Qoder bearer endpoint.
type Transport struct {
	httpClient *http.Client
	token      string
	endpoint   string
	baseURL    string
	timeout    time.Duration
	userAgent  string
}

// Config holds transport configuration.
type Config struct {
	HTTPClient        *http.Client
	Token             string // required: Bearer token
	BaseURL           string // test-only: override endpoint URL
	Timeout           time.Duration
	AllowTestEndpoint bool   // must be true when BaseURL is set
	UserAgent         string // override default UA
}

// NewTransport validates config and returns a Transport.
func NewTransport(cfg Config) (*Transport, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("bearer: token must not be empty")
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
	ua := cfg.UserAgent
	if ua == "" {
		ua = defaultUA
	}
	return &Transport{
		httpClient: cfg.HTTPClient,
		token:      cfg.Token,
		endpoint:   defaultEndpoint,
		baseURL:    cfg.BaseURL,
		timeout:    cfg.Timeout,
		userAgent:  ua,
	}, nil
}

// StreamRequest is the input for a streaming chat request.
type StreamRequest struct {
	Model               string
	Messages            []Message
	Tools               []Tool
	ToolChoice          json.RawMessage // string or structured tool_choice at the wire boundary
	Temperature         *float64
	MaxTokens           *int
	ReasoningEffort     *string
	MaxCompletionTokens *int
	ParallelToolCalls   *bool
	RequestID           string
	SessionID           string
}

// StreamResponse holds the SSE parser for a streaming response.
type StreamResponse struct {
	Context context.Context
	Cancel  context.CancelFunc
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

// Stream sends a request and returns an SSE parser.
func (t *Transport) Stream(ctx context.Context, req StreamRequest) (*StreamResponse, error) {
	if req.Model == "" {
		return nil, ErrEmptyModel
	}
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}
	body, err := buildRequestBody(req)
	if err != nil {
		return nil, fmt.Errorf("bearer: build request: %w", err)
	}
	url := t.endpoint
	if t.baseURL != "" {
		url = strings.TrimRight(t.baseURL, "/") + "/model/v1/chat/completions"
	}
	streamCtx, streamCancel := context.WithTimeout(ctx, t.timeout)
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		streamCancel()
		return nil, fmt.Errorf("bearer: create request: %w", err)
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+t.token)
	httpReq.Header.Set("Cache-Control", "no-cache")
	httpReq.Header.Set("Connection", "keep-alive")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", t.userAgent)
	httpReq.ContentLength = int64(len(body))

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		streamCancel()
		if streamCtx.Err() != nil {
			return nil, streamCtx.Err()
		}
		return nil, fmt.Errorf("bearer: HTTP request: %w", err)
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
	ct := resp.Header.Get("Content-Type")
	if !isTextEventStream(ct) {
		streamCancel()
		if err := drainAndClose(resp.Body); err != nil {
			return nil, fmt.Errorf("bearer: close invalid response: %w", err)
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

func buildRequestBody(req StreamRequest) ([]byte, error) {
	body := requestBody{
		Model:    req.Model,
		Messages: req.Messages,
		Stream:   true,
		StreamOptions: streamOptions{
			IncludeUsage: true,
		},
	}
	if len(req.Tools) > 0 {
		body.Tools = req.Tools
	}
	if len(req.ToolChoice) > 0 {
		body.ToolChoice = req.ToolChoice
	}
	if req.Temperature != nil {
		body.Temperature = req.Temperature
	}
	if req.MaxTokens != nil {
		body.MaxTokens = req.MaxTokens
	}
	if req.ReasoningEffort != nil {
		body.ReasoningEffort = req.ReasoningEffort
	}
	if req.MaxCompletionTokens != nil {
		body.MaxCompletionTokens = req.MaxCompletionTokens
	}
	if req.ParallelToolCalls != nil {
		body.ParallelToolCalls = req.ParallelToolCalls
	}
	if req.RequestID != "" || req.SessionID != "" {
		body.Metadata = requestMetadata{
			RequestID: req.RequestID,
			SessionID: req.SessionID,
		}
	}
	return json.Marshal(body)
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
