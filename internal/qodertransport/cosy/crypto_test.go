package cosy

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
)

// TestAESCBCEncryptDecrypt_RoundTrip verifies AES-128-CBC + PKCS7 round-trip.
func TestAESCBCEncryptDecrypt_RoundTrip(t *testing.T) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	plaintext := []byte(`{"uid":"test-user"}`)
	sealed, err := aesCBCEncrypt(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	decrypted, err := aesCBCDecrypt(sealed, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("round-trip: got %q, want %q", decrypted, plaintext)
	}
}

// TestPKCS7Pad_Unpad_RoundTrip verifies padding correctness.
func TestPKCS7Pad_Unpad_RoundTrip(t *testing.T) {
	blockSize := 16
	cases := [][]byte{
		{},
		{0x01},
		{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f},
		make([]byte, 16),
		make([]byte, 32),
	}
	for i, pt := range cases {
		padded := pkcs7Pad(pt, blockSize)
		if len(padded)%blockSize != 0 {
			t.Errorf("case %d: padded len %d not multiple of %d", i, len(padded), blockSize)
		}
		unpadded, err := pkcs7Unpad(padded, blockSize)
		if err != nil {
			t.Errorf("case %d: unpad: %v", i, err)
			continue
		}
		if !bytes.Equal(unpadded, pt) {
			t.Errorf("case %d: got %x, want %x", i, unpadded, pt)
		}
	}
}

// TestPKCS7Unpad_InvalidPadding rejects bad padding.
func TestPKCS7Unpad_InvalidPadding(t *testing.T) {
	bad := make([]byte, 16)
	_, err := pkcs7Unpad(bad, 16)
	if err == nil {
		t.Error("expected error for zero padding byte")
	}
	if !errors.Is(err, ErrInvalidPadding) {
		t.Errorf("error should wrap ErrInvalidPadding, got: %v", err)
	}
}

// TestPKCS7Unpad_TamperedPadding rejects tampered last byte.
func TestPKCS7Unpad_TamperedPadding(t *testing.T) {
	data := make([]byte, 16)
	data[15] = 0x04
	data[14] = 0x04
	data[13] = 0x03 // mismatch
	_, err := pkcs7Unpad(data, 16)
	if err == nil {
		t.Error("expected error for tampered padding")
	}
}

// TestRuntimeFieldsDerivation verifies AES+RSA envelope.
func TestRuntimeFieldsDerivation(t *testing.T) {
	entropy := &fixedEntropy{data: bytes.Repeat([]byte{0x41}, 256)}
	input := RuntimeFieldInput{UID: "test-uid", OrganizationTags: []string{}, DataPolicyAgreed: true}
	fields, err := DeriveRuntimeFields(entropy, input)
	if err != nil {
		t.Fatalf("DeriveRuntimeFields: %v", err)
	}
	if fields.EncryptUserInfo == "" || fields.Key == "" {
		t.Error("fields are empty")
	}
	if _, err := base64.StdEncoding.DecodeString(fields.EncryptUserInfo); err != nil {
		t.Errorf("EncryptUserInfo not valid base64: %v", err)
	}
}

// TestDeriveRuntimeFields_Deterministic verifies same entropy → same output.
func TestDeriveRuntimeFields_Deterministic(t *testing.T) {
	entropy := &fixedEntropy{data: bytes.Repeat([]byte{0x42}, 256)}
	input := RuntimeFieldInput{UID: "user-1", DataPolicyAgreed: true}
	a, _ := DeriveRuntimeFields(entropy, input)
	entropy.r = 0
	b, _ := DeriveRuntimeFields(entropy, input)
	if a.EncryptUserInfo != b.EncryptUserInfo || a.Key != b.Key {
		t.Error("non-deterministic derivation")
	}
}

// fixedEntropy is a deterministic reader for testing.
type fixedEntropy struct {
	data []byte
	r    int
}

func (f *fixedEntropy) Read(p []byte) (int, error) {
	n := copy(p, f.data[f.r:])
	f.r += n
	return n, nil
}

// TestRSAEncrypt_DecryptRoundTrip verifies RSA wrapping.
func TestRSAEncrypt_DecryptRoundTrip(t *testing.T) {
	priv, err := generateTestRSAKey()
	if err != nil {
		t.Skip("RSA key generation not available")
	}
	aesKey := []byte("0123456789abcdef")
	entropy := &fixedEntropy{data: bytes.Repeat([]byte{0x41}, 256)}
	wrapped, err := rsaEncryptPKCS1v15(entropy, &priv.PublicKey, aesKey)
	if err != nil {
		t.Fatalf("rsaEncrypt: %v", err)
	}
	if len(wrapped) != priv.Size() {
		t.Errorf("wrapped len: got %d, want %d", len(wrapped), priv.Size())
	}
}

// TestReverseMaskUUID verifies the UUID reversal and masking.
func TestReverseMaskUUID(t *testing.T) {
	raw := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	got := reverseMaskUUID(raw)
	if got[0] != 16 {
		t.Errorf("byte 0: got %d, want 16", got[0])
	}
	if got[6] != 0x4a {
		t.Errorf("byte 6: got 0x%02x, want 0x4a", got[6])
	}
}

// TestRuntimeASCIIKey verifies hex encoding.
func TestRuntimeASCIIKey(t *testing.T) {
	uuid := [16]byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0}
	got := runtimeASCIIKey(uuid)
	want := "123456789abcdef0"
	if string(got) != want {
		t.Errorf("runtimeASCIIKey: got %q, want %q", got, want)
	}
}

// TestRSAEncryptRejectsOversizedKey verifies error for oversized input.
func TestRSAEncryptRejectsOversizedKey(t *testing.T) {
	priv, err := generateTestRSAKey()
	if err != nil {
		t.Skip("RSA key generation not available")
	}
	tooLong := make([]byte, 200)
	entropy := &fixedEntropy{data: bytes.Repeat([]byte{0x41}, 256)}
	_, err = rsaEncryptPKCS1v15(entropy, &priv.PublicKey, tooLong)
	if err == nil {
		t.Error("expected error for oversized RSA input")
	}
	if !errors.Is(err, ErrRSAKeyTooLong) {
		t.Errorf("error should wrap ErrRSAKeyTooLong, got: %v", err)
	}
}

// TestAESCBCEncryptRejectsWrongKeyLen verifies key length validation.
func TestAESCBCEncryptRejectsWrongKeyLen(t *testing.T) {
	_, err := aesCBCEncrypt([]byte("test"), make([]byte, 8))
	if err == nil {
		t.Error("expected error for 8-byte key")
	}
	if !errors.Is(err, ErrAESKeyLength) {
		t.Errorf("error should wrap ErrAESKeyLength, got: %v", err)
	}
}

// TestAESCBCEncryptRejectsEmptyKey verifies zero-length key rejection.
func TestAESCBCEncryptRejectsEmptyKey(t *testing.T) {
	_, err := aesCBCEncrypt([]byte("test"), nil)
	if err == nil {
		t.Error("expected error for nil key")
	}
	if !errors.Is(err, ErrAESKeyLength) {
		t.Errorf("error should wrap ErrAESKeyLength, got: %v", err)
	}
}

// TestAESCBCDecryptRejectsNotMultipleOfBlockSize verifies non-aligned ciphertext.
func TestAESCBCDecryptRejectsNotMultipleOfBlockSize(t *testing.T) {
	_, err := aesCBCDecrypt(make([]byte, 15), make([]byte, 16))
	if err == nil {
		t.Error("expected error for non-block-aligned ciphertext")
	}
	if !errors.Is(err, ErrAESBlockSize) {
		t.Errorf("error should be ErrAESBlockSize, got: %v", err)
	}
}
