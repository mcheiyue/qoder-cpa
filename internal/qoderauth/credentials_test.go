package qoderauth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewAuthID_Deterministic(t *testing.T) {
	uid := "user-123"
	id1 := newAuthID(uid)
	id2 := newAuthID(uid)
	if id1 != id2 {
		t.Fatalf("AuthID not deterministic: %q != %q", id1, id2)
	}
	if !strings.HasPrefix(string(id1), "qoder-") {
		t.Fatalf("AuthID missing prefix: %q", id1)
	}
	h := sha256.Sum256([]byte(uid))
	want := "qoder-" + hexEncode(h[:])
	if string(id1) != want {
		t.Fatalf("AuthID=%q, want %q", id1, want)
	}
}

func TestNewAuthID_DifferentInputs(t *testing.T) {
	id1 := newAuthID("alice")
	id2 := newAuthID("bob")
	if id1 == id2 {
		t.Fatal("different user IDs produced same AuthID")
	}
}

func TestHexEncode(t *testing.T) {
	input := []byte{0x0a, 0xbc, 0xff, 0x00}
	got := hexEncode(input)
	if got != "0abcff00" {
		t.Fatalf("hexEncode=%q, want %q", got, "0abcff00")
	}
}

func TestPKCEVerifier_RandomLength(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		v, err := newPKCEVerifier()
		if err != nil {
			t.Fatalf("newPKCEVerifier: %v", err)
		}
		if len(v) != 43 {
			t.Fatalf("verifier len=%d, want 43", len(v))
		}
		for _, c := range v {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
				t.Fatalf("invalid base64url char: %c", c)
			}
		}
		if seen[v] {
			t.Fatalf("duplicate verifier: %q", v)
		}
		seen[v] = true
	}
}

func TestPKCEChallenge_Deterministic(t *testing.T) {
	verifier := "test-verifier-string-for-s256-challenge"
	c1 := pkceChallenge(verifier)
	c2 := pkceChallenge(verifier)
	if c1 != c2 {
		t.Fatalf("challenge not deterministic: %q != %q", c1, c2)
	}
	h := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(h[:])
	if c1 != want {
		t.Fatalf("challenge=%q, want %q", c1, want)
	}
}

func TestPKCEChallenge_DifferentVerifiers(t *testing.T) {
	c1 := pkceChallenge("verifier-a")
	c2 := pkceChallenge("verifier-b")
	if c1 == c2 {
		t.Fatal("different verifiers produced same challenge")
	}
}

func TestNewNonce_RandomLength(t *testing.T) {
	for i := 0; i < 20; i++ {
		n, err := newNonce()
		if err != nil {
			t.Fatalf("newNonce: %v", err)
		}
		if len(n) < 21 || len(n) > 24 {
			t.Fatalf("nonce len=%d, want ~22", len(n))
		}
	}
}

func TestNewNonce_Unique(t *testing.T) {
	n1, _ := newNonce()
	n2, _ := newNonce()
	if n1 == n2 {
		t.Fatal("two nonces are identical")
	}
}

func TestNewMachineID_Format(t *testing.T) {
	for i := 0; i < 10; i++ {
		m, err := newMachineID()
		if err != nil {
			t.Fatalf("newMachineID: %v", err)
		}
		if !strings.HasPrefix(string(m), "m-") {
			t.Fatalf("machine ID missing m- prefix: %q", m)
		}
		hex := strings.TrimPrefix(string(m), "m-")
		if len(hex) != 32 {
			t.Fatalf("machine ID hex len=%d, want 32", len(hex))
		}
	}
}

func TestFromCredential_Roundtrip(t *testing.T) {
	cred := Credential{
		AccessToken:  "at-abc",
		RefreshToken: "rt-def",
		ExpiresAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UserID:       "u123",
		Email:        "test@qoder.com",
		Profile:      TransportProfileCosyAPI2,
	}
	sj := FromCredential(cred)
	back := sj.ToCredential()
	if back.AccessToken != cred.AccessToken {
		t.Fatalf("AccessToken: %q != %q", back.AccessToken, cred.AccessToken)
	}
	if back.RefreshToken != cred.RefreshToken {
		t.Fatalf("RefreshToken: %q != %q", back.RefreshToken, cred.RefreshToken)
	}
	if !back.ExpiresAt.Equal(cred.ExpiresAt) {
		t.Fatalf("ExpiresAt: %v != %v", back.ExpiresAt, cred.ExpiresAt)
	}
	if back.UserID != cred.UserID {
		t.Fatalf("UserID: %q != %q", back.UserID, cred.UserID)
	}
	if back.Email != cred.Email {
		t.Fatalf("Email: %q != %q", back.Email, cred.Email)
	}
	if back.Profile != cred.Profile {
		t.Fatalf("Profile: %q != %q", back.Profile, cred.Profile)
	}
}

func TestStorageJSON_JSONNoSecrets(t *testing.T) {
	sj := StorageJSON{
		AccessToken:  "secret-at",
		RefreshToken: "secret-rt",
		ExpiresAt:    time.Now(),
		UserID:       "u1",
		Email:        "x@qoder.com",
		Profile:      TransportProfileBearerOpenAI,
	}
	raw, err := json.Marshal(sj)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, forbidden := range []string{"verifier", "nonce", "device_code", "verify_url", "transaction"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("StorageJSON JSON contains forbidden key %q: %s", forbidden, s)
		}
	}
}

func TestCredential_Validate_EmptyAccessToken(t *testing.T) {
	cred := Credential{UserID: "u1"}
	if err := cred.Validate(); err == nil {
		t.Fatal("expected error for empty access token")
	}
}

func TestCredential_Validate_EmptyUserID(t *testing.T) {
	cred := Credential{AccessToken: "at"}
	if err := cred.Validate(); err == nil {
		t.Fatal("expected error for empty user ID")
	}
}

func TestCredential_Validate_Valid(t *testing.T) {
	cred := Credential{AccessToken: "at", UserID: "u1"}
	if err := cred.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsValidProfile(t *testing.T) {
	tests := []struct {
		profile TransportProfile
		valid   bool
	}{
		{TransportProfileCosyAPI2, true},
		{TransportProfileCosyAPI3, true},
		{TransportProfileBearerOpenAI, true},
		{"unknown", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := IsValidProfile(tc.profile); got != tc.valid {
			t.Errorf("IsValidProfile(%q) = %v, want %v", tc.profile, got, tc.valid)
		}
	}
}

func TestDrainAndClose_NilBody(t *testing.T) {
	drainAndClose(nil)
}
