package cosy

import (
	"crypto/rand"
	"testing"
)

// TestEncodeDecode_RoundTrip verifies that encode then decode recovers the original.
func TestEncodeDecode_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"short", []byte("hello")},
		{"json", []byte(`{"key":"value","num":42}`)},
		{"binary", make([]byte, 256)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "binary" {
				if _, err := rand.Read(tc.data); err != nil {
					t.Fatalf("rand.Read: %v", err)
				}
			}
			encoded := EncodeBody(tc.data)
			decoded := DecodeBody(encoded)
			if string(decoded) != string(tc.data) {
				t.Errorf("round-trip failed: got %q, want %q", decoded, tc.data)
			}
		})
	}
}

// TestSwapOuterThirds_Vectors checks known swap positions.
func TestSwapOuterThirds_Vectors(t *testing.T) {
	// "ABCDEFGHIJ" (len=10, q=3): last 3 + middle 4 + first 3
	in := []byte("ABCDEFGHIJ")
	got := swapOuterThirds(in)
	want := []byte("HIJDEFGABC")
	if string(got) != string(want) {
		t.Errorf("swapOuterThirds(%q) = %q, want %q", in, got, want)
	}
}

// TestEncodeBody_Deterministic verifies same input → same output.
func TestEncodeBody_Deterministic(t *testing.T) {
	data := []byte(`{"test":"deterministic"}`)
	a := EncodeBody(data)
	b := EncodeBody(data)
	if string(a) != string(b) {
		t.Errorf("non-deterministic: %q != %q", a, b)
	}
}

// TestDecodeBody_InvalidAlphabet rejects characters outside the private alphabet.
func TestDecodeBody_InvalidAlphabet(t *testing.T) {
	// Standard base64 chars not in private alphabet should fail strict decode.
	_, err := decodePrivate([]byte("ABCD+EFG/"))
	if err == nil {
		t.Error("expected error for invalid alphabet characters")
	}
}
