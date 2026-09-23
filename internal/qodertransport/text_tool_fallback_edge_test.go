package qodertransport

import (
	"io"
	"strings"
	"testing"
)

func TestTextToolFallback_OversizeBuffer(t *testing.T) {
	pad := strings.Repeat("x", 1024)
	big := "Tool calls: " + pad
	for len(big) < maxToolFallbackBuf+1 {
		big += pad
	}
	inner := &seqHandle{chunks: [][]byte{textD("c1", "m", big), finishD("c1", "m", "stop"), doneSSE()}}
	chunks, err := collect(newTextToolFallback(inner, true, "c1", "m"))
	if err != nil {
		t.Fatal(err)
	}
	if hasSub(chunks, `"tool_calls"`) {
		t.Fatalf("oversize must not emit tool_calls, got %d chunks", len(chunks))
	}
}

func TestTextToolFallback_ErrorPreservesBuffer(t *testing.T) {
	inner := &errHandle{chunks: [][]byte{textD("c1", "m", "buffered text")}, err: io.ErrUnexpectedEOF}
	chunks, err := collect(newTextToolFallback(inner, true, "c1", "m"))
	if !hasSub(chunks, `"content":"buffered text"`) {
		t.Fatalf("buffered text must be preserved, chunks=%q err=%v", chunks, err)
	}
}

func TestTextToolFallback_DonePreservesIncompleteBuffer(t *testing.T) {
	inner := &seqHandle{chunks: [][]byte{textD("c1", "m", "Tool calls: ["), doneSSE()}}
	chunks, err := collect(newTextToolFallback(inner, true, "c1", "m"))
	if err != nil {
		t.Fatal(err)
	}
	if !hasSub(chunks, `"content":"Tool calls: ["`) {
		t.Fatalf("incomplete buffer must be preserved, chunks=%q", chunks)
	}
}

func TestTextToolFallback_EOFWithEmptyBuffer(t *testing.T) {
	inner := &seqHandle{chunks: [][]byte{textD("c1", "m", "hello"), finishD("c1", "m", "stop"), doneSSE()}}
	chunks, err := collect(newTextToolFallback(inner, true, "c1", "m"))
	if err != nil {
		t.Fatal(err)
	}
	if !hasSub(chunks, `"content":"hello"`) || !hasSub(chunks, `"finish_reason":"stop"`) {
		t.Fatalf("normal stream must pass through, got: %q", chunks)
	}
}

func TestTextToolFallback_CancelDelegates(t *testing.T) {
	h := newTextToolFallback(&seqHandle{}, true, "c1", "m")
	h.Cancel()
}

func TestTextToolFallback_SingleDeltaComplete(t *testing.T) {
	inner := &seqHandle{chunks: [][]byte{
		textD("c1", "m", `Tool calls: [{"id":"c1","type":"function","function":{"name":"go","arguments":"{}"}}]`),
		finishD("c1", "m", "stop"), doneSSE(),
	}}
	chunks, err := collect(newTextToolFallback(inner, true, "c1", "m"))
	if err != nil {
		t.Fatal(err)
	}
	if !hasSub(chunks, `"tool_calls"`) || !hasSub(chunks, `"name":"go"`) {
		t.Fatalf("single delta tool calls, got: %q", chunks)
	}
}
