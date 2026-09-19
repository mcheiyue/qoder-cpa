package cosy

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// BuildCOSYPayload renders the JSON payload and its standard base64 form.
func BuildCOSYPayload(requestID, info, cosyVersion string) (string, error) {
	raw, err := json.Marshal(map[string]string{
		"version":     "v1",
		"requestId":   requestID,
		"info":        info,
		"cosyVersion": cosyVersion,
		"ideVersion":  "",
	})
	if err != nil {
		return "", fmt.Errorf("marshal cosy payload: %w", err)
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// SignRequest computes the MD5 signature of five newline-separated fields.
func SignRequest(payloadBase64, runtimeKey, unixSeconds, encodedBody, signedPath string) string {
	data := payloadBase64 + "\n" + runtimeKey + "\n" + unixSeconds + "\n" + encodedBody + "\n" + signedPath
	h := md5.Sum([]byte(data))
	return hex.EncodeToString(h[:])
}

// ComposeBearer renders the Authorization header value.
func ComposeBearer(payloadBase64, signature string) string {
	return "Bearer COSY." + payloadBase64 + "." + signature
}

// SignPath extracts the path component for signature: strips /algo prefix and query.
func SignPath(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	idx := strings.Index(rawURL, "://")
	if idx < 0 {
		return rawURL
	}
	rest := rawURL[idx+3:]
	slashIdx := strings.Index(rest, "/")
	if slashIdx < 0 {
		return "/"
	}
	path := rest[slashIdx:]
	qIdx := strings.Index(path, "?")
	if qIdx >= 0 {
		path = path[:qIdx]
	}
	if strings.HasPrefix(path, "/algo") {
		path = strings.TrimPrefix(path, "/algo")
	}
	return path
}
