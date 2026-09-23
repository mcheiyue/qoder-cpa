package qodertransport

import (
	"bytes"
	"encoding/json"
)

func bufferMatchesPrefix(buf []byte) bool {
	if !bytes.HasPrefix(buf, []byte("Tool calls:")) {
		return false
	}
	rest := bytes.TrimLeft(buf[len("Tool calls:"):], " \t\r\n")
	return len(rest) == 0 || rest[0] == '[' || rest[0] == '`'
}

func tryParseToolCalls(buf []byte) ([]toolCallInfo, bool) {
	rest := bytes.TrimSpace(bytes.TrimPrefix(buf, []byte("Tool calls:")))
	if bytes.HasPrefix(rest, []byte("```json")) {
		rest = rest[len("```json"):]
		if idx := bytes.Index(rest, []byte("```")); idx >= 0 {
			rest = rest[:idx]
		}
		rest = bytes.TrimSpace(rest)
	}
	if len(rest) == 0 {
		return nil, false
	}
	var calls []toolCallInfo
	if json.Unmarshal(rest, &calls) != nil || len(calls) == 0 || len(calls) > maxToolFallbackCalls {
		return nil, false
	}
	return calls, true
}
