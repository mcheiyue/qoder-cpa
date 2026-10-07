package qodertransport

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/cosy"
)

// --- response_format pass-through (Orchids e92b35c2) ---

func TestParseChatPayload_ResponseFormat(t *testing.T) {
	raw := minimalJSON(`"response_format":{"type":"json_object"}`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	if len(p.ResponseFormat) == 0 {
		t.Fatal("ResponseFormat should be captured from payload")
	}
	var m map[string]any
	if err := json.Unmarshal(p.ResponseFormat, &m); err != nil {
		t.Fatalf("ResponseFormat not valid JSON: %v", err)
	}
	if m["type"] != "json_object" {
		t.Errorf("type: got %v, want json_object", m["type"])
	}
}

func TestToCosyRequest_ResponseFormatJsonObject(t *testing.T) {
	raw := minimalJSON(`"response_format":{"type":"json_object"}`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.Parameters == nil {
		t.Fatal("Parameters should allocate when response_format set")
	}
	if len(c.Parameters.ResponseFormat) == 0 {
		t.Fatal("ResponseFormat should pass through to COSY Parameters")
	}
	var m map[string]any
	if err := json.Unmarshal(c.Parameters.ResponseFormat, &m); err != nil {
		t.Fatalf("passthrough not valid JSON: %v", err)
	}
	if m["type"] != "json_object" {
		t.Errorf("type: got %v, want json_object", m["type"])
	}
}

func TestToCosyRequest_ResponseFormatJsonSchemaRawPreserved(t *testing.T) {
	// json_schema envelope must survive byte-for-byte (no re-shaping by us).
	const schema = `{"type":"json_schema","json_schema":{"name":"resp","strict":true,"schema":{"type":"object"}}}`
	raw := minimalJSON(`"response_format":` + schema)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.Parameters == nil {
		t.Fatal("Parameters should allocate when response_format set")
	}
	if got := string(c.Parameters.ResponseFormat); got != schema {
		t.Errorf("json_schema passthrough changed bytes:\n got %s\nwant %s", got, schema)
	}
}

func TestToCosyRequest_ResponseFormatAbsentNoDrift(t *testing.T) {
	// F0b guard: absent response_format must not appear on the wire at all.
	raw := minimalJSON(`"max_tokens":100`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.Parameters == nil {
		t.Fatal("Parameters should allocate for max_tokens")
	}
	if len(c.Parameters.ResponseFormat) != 0 {
		t.Errorf("ResponseFormat should stay empty when absent, got %s", c.Parameters.ResponseFormat)
	}
	serialized, err := json.Marshal(c.Parameters)
	if err != nil {
		t.Fatalf("marshal parameters: %v", err)
	}
	if strings.Contains(string(serialized), "response_format") {
		t.Errorf("absent response_format leaked onto wire: %s", serialized)
	}
}

func TestCosyParameters_SerialisationResponseFormatOmitted(t *testing.T) {
	p := cosy.Parameters{}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "response_format") {
		t.Errorf("empty Parameters leaked response_format: %s", raw)
	}
}
