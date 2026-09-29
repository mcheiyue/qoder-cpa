package cosy

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func f2WireInput() BuildRequestInput {
	return BuildRequestInput{
		RequestID: "req-1",
		SessionID: "sess-1",
		ModelKey:  "qfmodel",
		BeginAt:   time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
		Messages:  []ChatMessageIn{{Role: "user", Content: "hi"}},
	}
}

// F2 default behaviour must stay byte-identical to the current production
// wire: session_type=qodercli, and no aliyun_user_type key at all.
func TestBuildChatBodyDefaultKeepsProductionWire(t *testing.T) {
	raw, err := BuildChatBody(f2WireInput())
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body["session_type"] != "qodercli" {
		t.Fatalf("session_type=%v, want qodercli", body["session_type"])
	}
	if _, ok := body["aliyun_user_type"]; ok {
		t.Fatalf("aliyun_user_type must be absent by default, got %s", raw)
	}
}

// F2 explicit overrides reach the wire with the requested values.
func TestBuildChatBodySessionAndUserTypeOverride(t *testing.T) {
	in := f2WireInput()
	in.SessionType = "qoder"
	in.AliyunUserType = "personal_standard"
	raw, err := BuildChatBody(in)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body["session_type"] != "qoder" {
		t.Fatalf("session_type=%v, want qoder", body["session_type"])
	}
	if body["aliyun_user_type"] != "personal_standard" {
		t.Fatalf("aliyun_user_type=%v, want personal_standard", body["aliyun_user_type"])
	}
}

// F2 empty-value policy: an empty AliyunUserType serializes to no key, so a
// request built without a real account class is indistinguishable from today.
func TestBuildChatBodyEmptyUserTypeStaysAbsent(t *testing.T) {
	in := f2WireInput()
	in.SessionType = "qodercli"
	in.AliyunUserType = ""
	raw, err := BuildChatBody(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "aliyun_user_type") {
		t.Fatalf("empty aliyun_user_type must stay absent, got %s", raw)
	}
}

// F2 signature coverage: aliyun_user_type lives inside the signed encoded
// body, so flipping it must change the computed signature.
func TestAliyunUserTypeCoveredBySignature(t *testing.T) {
	base, err := BuildChatBody(f2WireInput())
	if err != nil {
		t.Fatal(err)
	}
	with := f2WireInput()
	with.AliyunUserType = "personal_standard"
	alt, err := BuildChatBody(with)
	if err != nil {
		t.Fatal(err)
	}
	sigBase := SignRequest("payload", "key", "1700000000", string(EncodeBody(base)), "/algo/path")
	sigAlt := SignRequest("payload", "key", "1700000000", string(EncodeBody(alt)), "/algo/path")
	if sigBase == sigAlt {
		t.Fatal("aliyun_user_type is not covered by the signature")
	}
	// The field must be inside the encoded body that gets signed.
	decoded := DecodeBody(EncodeBody(alt))
	if !strings.Contains(string(decoded), "aliyun_user_type") {
		t.Fatalf("field missing from signed body: %s", decoded)
	}
}
