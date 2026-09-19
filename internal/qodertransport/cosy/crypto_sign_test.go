package cosy

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"
)

// TestSignRequest_GoldVector verifies the MD5 signature.
func TestSignRequest_GoldVector(t *testing.T) {
	payload := "dGVzdHBheWxvYWQ="
	key := "dGVzdGtleQ=="
	seconds := "1234567890"
	body := "encodedbody"
	path := "/test/path"
	sig := SignRequest(payload, key, seconds, body, path)
	const want = "8f9334c940deaeb98230a004df6544dc"
	if sig != want {
		t.Errorf("signature: got %q, want %q", sig, want)
	}
	if _, err := hex.DecodeString(sig); err != nil {
		t.Errorf("signature not valid hex: %v", err)
	}
}

// TestComposeBearer verifies the Bearer token format.
func TestComposeBearer(t *testing.T) {
	got := ComposeBearer("payloadb64", "sig123")
	want := "Bearer COSY.payloadb64.sig123"
	if got != want {
		t.Errorf("ComposeBearer: got %q, want %q", got, want)
	}
}

// TestBuildCOSYPayload verifies JSON structure and base64 encoding.
func TestBuildCOSYPayload(t *testing.T) {
	payload, err := BuildCOSYPayload("req-123", "encryptedinfo", "1.1.34")
	if err != nil {
		t.Fatalf("BuildCOSYPayload: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("not valid base64: %v", err)
	}
	var raw map[string]string
	if err := json.Unmarshal(decoded, &raw); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if raw["version"] != "v1" {
		t.Errorf("version: got %q, want %q", raw["version"], "v1")
	}
	if raw["requestId"] != "req-123" {
		t.Errorf("requestId: got %q", raw["requestId"])
	}
	if raw["info"] != "encryptedinfo" {
		t.Errorf("info: got %q", raw["info"])
	}
	if raw["cosyVersion"] != "1.1.34" {
		t.Errorf("cosyVersion: got %q", raw["cosyVersion"])
	}
}

// TestSignPath verifies path extraction for signature.
func TestSignPath(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://api2.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result", "/api/v2/service/pro/sse/agent_chat_generation"},
		{"https://api2.qoder.sh/api/v2/service/pro/sse/agent_chat_generation", "/api/v2/service/pro/sse/agent_chat_generation"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			got := SignPath(tc.url)
			if got != tc.want {
				t.Errorf("SignPath(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

// TestSignConstantTime verifies deterministic signature.
func TestSignConstantTime(t *testing.T) {
	a := SignRequest("p", "k", "1", "b", "/")
	b := SignRequest("p", "k", "1", "b", "/")
	if a != b {
		t.Error("same inputs should produce same signature")
	}
	c := SignRequest("p", "k", "1", "b", "/other")
	if a == c {
		t.Error("different inputs should produce different signatures")
	}
}

// TestEmptyBodyEncoding verifies empty input encoding.
func TestEmptyBodyEncoding(t *testing.T) {
	encoded := EncodeBody([]byte{})
	decoded := DecodeBody(encoded)
	if len(decoded) != 0 {
		t.Errorf("empty body round-trip: got %d bytes, want 0", len(decoded))
	}
}

// TestLargeBodyEncoding verifies >64KiB body works.
func TestLargeBodyEncoding(t *testing.T) {
	data := make([]byte, 128*1024)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	encoded := EncodeBody(data)
	decoded := DecodeBody(encoded)
	if !bytes.Equal(decoded, data) {
		t.Error("large body round-trip failed")
	}
}
