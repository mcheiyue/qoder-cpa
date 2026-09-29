package qoderauth

import (
	"encoding/json"
	"strings"
	"testing"
)

// F2: UserType must round-trip through the persisted StorageJSON so the
// account class recorded from quota/profile survives a restart.
func TestStorageJSONUserTypeRoundTrip(t *testing.T) {
	stored := StorageJSON{
		AccessToken: "at",
		UserID:      "u1",
		UserType:    "personal_standard",
		Profile:     TransportProfileCosyAPI2,
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	var back StorageJSON
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.UserType != "personal_standard" {
		t.Fatalf("user_type=%q", back.UserType)
	}
	cred := back.ToCredential()
	if cred.UserType != "personal_standard" {
		t.Fatalf("credential UserType=%q", cred.UserType)
	}
	again := FromCredential(cred)
	if again.UserType != "personal_standard" {
		t.Fatalf("FromCredential UserType=%q", again.UserType)
	}
}

// F2 empty-value policy: a missing account class must be recorded as absent,
// never serialized as an empty string or a static default.
func TestStorageJSONUserTypeOmittedWhenMissing(t *testing.T) {
	raw, err := json.Marshal(StorageJSON{AccessToken: "at", UserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "user_type") {
		t.Fatalf("missing user_type must stay absent, got %s", raw)
	}
	var back StorageJSON
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.UserType != "" {
		t.Fatalf("user_type=%q, want empty (missing)", back.UserType)
	}
}
