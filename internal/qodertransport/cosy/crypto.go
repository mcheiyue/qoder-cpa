package cosy

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"
)

// Typed errors for crypto operations. No secret data in messages.
var (
	ErrAESKeyLength        = errors.New("cosy: AES key must be 16 bytes")
	ErrAESBlockSize        = errors.New("cosy: ciphertext not multiple of block size")
	ErrInvalidPadding      = errors.New("cosy: invalid PKCS#7 padding")
	ErrRSAKeyTooLong       = errors.New("cosy: message too long for RSA key")
	ErrRSAEntropyExhausted = errors.New("cosy: RSA padding entropy exhausted")
)

// runtimePublicKeyPEM is the gateway's runtime field public key.
// Non-secret: distributed inside the Qoder CLI.
const runtimePublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDA8iMH5c02LilrsERw9t6Pv5Nc
4k6Pz1EaDicBMpdpxKduSZu5OANqUq8er4GM95omAGIOPOh+Nx0spthYA2BqGz+l
6HRkPJ7S236FZz73In/KVuLnwI8JJ2CbuJap8kvheCCZpmAWpb/cPx/3Vr/J6I17
XcW+ML9FoCI6AOvOzwIDAQAB
-----END PUBLIC KEY-----`

// RuntimeFieldInput is the exact AES plaintext for runtime fields.
type RuntimeFieldInput struct {
	UID              string   `json:"uid"`
	OrganizationID   string   `json:"organization_id"`
	OrganizationTags []string `json:"organization_tags"`
	DataPolicyAgreed bool     `json:"data_policy_agreed"`
}

// RuntimeFields holds the derived per-account authentication pair.
type RuntimeFields struct {
	EncryptUserInfo string
	Key             string
}

// DeriveRuntimeFields derives the AES+RSA pair for one account.
func DeriveRuntimeFields(entropy io.Reader, in RuntimeFieldInput) (RuntimeFields, error) {
	if entropy == nil {
		entropy = rand.Reader
	}
	if in.OrganizationTags == nil {
		in.OrganizationTags = []string{}
	}
	var raw [16]byte
	if _, err := io.ReadFull(entropy, raw[:]); err != nil {
		return RuntimeFields{}, fmt.Errorf("read runtime field entropy: %w", err)
	}
	key := runtimeASCIIKey(reverseMaskUUID(raw))

	plaintext, err := json.Marshal(in)
	if err != nil {
		return RuntimeFields{}, fmt.Errorf("marshal runtime fields: %w", err)
	}
	sealed, err := aesCBCEncrypt(plaintext, key)
	if err != nil {
		return RuntimeFields{}, err
	}
	publicKey, err := runtimePublicKey()
	if err != nil {
		return RuntimeFields{}, err
	}
	wrapped, err := rsaEncryptPKCS1v15(entropy, publicKey, key)
	if err != nil {
		return RuntimeFields{}, fmt.Errorf("wrap runtime key: %w", err)
	}
	return RuntimeFields{
		EncryptUserInfo: base64.StdEncoding.EncodeToString(sealed),
		Key:             base64.StdEncoding.EncodeToString(wrapped),
	}, nil
}

// reverseMaskUUID turns 16 random bytes into a UUID-like value by reversing
// and applying RFC 4122 version/variant masks.
func reverseMaskUUID(raw [16]byte) [16]byte {
	var out [16]byte
	for i := range raw {
		out[i] = raw[15-i]
	}
	out[6] = (out[6] & 0x0f) | 0x40
	out[8] = (out[8] & 0x3f) | 0x80
	return out
}

// runtimeASCIIKey is the AES key: lowercase hex of first 8 UUID bytes, kept as 16 ASCII chars.
func runtimeASCIIKey(value [16]byte) []byte {
	encoded := make([]byte, 16)
	hex.Encode(encoded, value[:8])
	return encoded
}

// aesCBCEncrypt encrypts with AES-128-CBC, key doubling as IV, with PKCS#7 padding.
func aesCBCEncrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAESKeyLength, err)
	}
	padded := pkcs7Pad(plaintext, block.BlockSize())
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, key[:block.BlockSize()]).CryptBlocks(out, padded)
	return out, nil
}

// aesCBCDecrypt decrypts AES-128-CBC with PKCS#7 unpadding.
func aesCBCDecrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAESKeyLength, err)
	}
	if len(ciphertext)%block.BlockSize() != 0 {
		return nil, ErrAESBlockSize
	}
	out := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, key[:block.BlockSize()]).CryptBlocks(out, ciphertext)
	return pkcs7Unpad(out, block.BlockSize())
}

func pkcs7Pad(plaintext []byte, blockSize int) []byte {
	padding := blockSize - len(plaintext)%blockSize
	out := make([]byte, len(plaintext)+padding)
	copy(out, plaintext)
	for i := len(plaintext); i < len(out); i++ {
		out[i] = byte(padding)
	}
	return out
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, ErrInvalidPadding
	}
	pad := int(data[len(data)-1])
	if pad == 0 || pad > blockSize {
		return nil, fmt.Errorf("%w: byte value %d", ErrInvalidPadding, pad)
	}
	for i := len(data) - pad; i < len(data); i++ {
		if data[i] != byte(pad) {
			return nil, fmt.Errorf("%w: mismatch at byte %d", ErrInvalidPadding, i)
		}
	}
	return data[:len(data)-pad], nil
}

// rsaMaxPaddingRetries is the maximum number of zero-byte retries during
// PKCS#1 v1.5 padding. Prevents infinite loop when entropy is depleted.
const rsaMaxPaddingRetries = 1024

// rsaEncryptPKCS1v15 implements PKCS#1 v1.5 with explicit non-zero padding.
func rsaEncryptPKCS1v15(entropy io.Reader, publicKey *rsa.PublicKey, message []byte) ([]byte, error) {
	size := publicKey.Size()
	if len(message) > size-11 {
		return nil, fmt.Errorf("%w: %d bytes for %d-byte key", ErrRSAKeyTooLong, len(message), size)
	}
	block := make([]byte, size)
	block[0] = 0x00
	block[1] = 0x02
	padding := size - len(message) - 3
	retries := 0
	for i := 0; i < padding; {
		var one [1]byte
		if _, err := io.ReadFull(entropy, one[:]); err != nil {
			return nil, err
		}
		if one[0] == 0 {
			retries++
			if retries > rsaMaxPaddingRetries {
				return nil, ErrRSAEntropyExhausted
			}
			continue
		}
		retries = 0
		block[2+i] = one[0]
		i++
	}
	block[2+padding] = 0x00
	copy(block[2+padding+1:], message)

	encrypted := new(big.Int).Exp(new(big.Int).SetBytes(block), big.NewInt(int64(publicKey.E)), publicKey.N).Bytes()
	if len(encrypted) < size {
		padded := make([]byte, size-len(encrypted), size)
		encrypted = append(padded, encrypted...)
	}
	return encrypted, nil
}

var (
	runtimeKeyOnce sync.Once
	runtimeKey     *rsa.PublicKey
	runtimeKeyErr  error
)

func runtimePublicKey() (*rsa.PublicKey, error) {
	runtimeKeyOnce.Do(func() {
		block, _ := pem.Decode([]byte(runtimePublicKeyPEM))
		if block == nil {
			runtimeKeyErr = fmt.Errorf("pinned runtime public key is not PEM")
			return
		}
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			runtimeKeyErr = fmt.Errorf("parse pinned runtime public key: %w", err)
			return
		}
		key, ok := parsed.(*rsa.PublicKey)
		if !ok {
			runtimeKeyErr = fmt.Errorf("pinned runtime public key is %T, want RSA", parsed)
			return
		}
		runtimeKey = key
	})
	return runtimeKey, runtimeKeyErr
}

func generateTestRSAKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, 1024)
}
