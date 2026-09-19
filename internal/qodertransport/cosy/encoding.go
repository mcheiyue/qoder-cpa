package cosy

import (
	"encoding/base64"
	"fmt"
)

// bodyAlphabet is the private base64 alphabet the Qoder gateway expects.
const bodyAlphabet = "_doRTgHZBKcGVjlvpC,@aFSx#DPuNJme&i*MzLOEn)sUrthbf%Y^w.(kIQyXqWA!"

// bodyPadding substitutes for '=' in the private alphabet.
const bodyPadding = '$'

// bodyEncoding is the strict private-alphabet encoder.
var bodyEncoding = base64.NewEncoding(bodyAlphabet).WithPadding(bodyPadding).Strict()

// EncodeBody renders raw bytes using the private alphabet and outer-third swap.
func EncodeBody(raw []byte) []byte {
	encoded := make([]byte, bodyEncoding.EncodedLen(len(raw)))
	bodyEncoding.Encode(encoded, raw)
	return swapOuterThirds(encoded)
}

// DecodeBody reverses the outer-third swap and private-alphabet decoding.
func DecodeBody(encoded []byte) []byte {
	swapped := swapOuterThirds(encoded)
	decoded := make([]byte, bodyEncoding.DecodedLen(len(swapped)))
	n, err := bodyEncoding.Decode(decoded, swapped)
	if err != nil {
		return nil
	}
	return decoded[:n]
}

// decodePrivate decodes with the private alphabet without the swap.
func decodePrivate(encoded []byte) ([]byte, error) {
	decoded := make([]byte, bodyEncoding.DecodedLen(len(encoded)))
	n, err := bodyEncoding.Decode(decoded, encoded)
	if err != nil {
		return nil, fmt.Errorf("private alphabet decode: %w", err)
	}
	return decoded[:n], nil
}

// swapOuterThirds moves the trailing third to the front, the middle stays,
// and the leading third moves to the end. Middle absorbs remainder.
func swapOuterThirds(src []byte) []byte {
	q := len(src) / 3
	out := make([]byte, 0, len(src))
	out = append(out, src[len(src)-q:]...)
	out = append(out, src[q:len(src)-q]...)
	out = append(out, src[:q]...)
	return out
}
