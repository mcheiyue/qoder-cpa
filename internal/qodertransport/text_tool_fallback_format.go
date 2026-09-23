package qodertransport

import (
	"encoding/json"
	"strings"
)

func buildToolCallChunks(id, model string, calls []toolCallInfo) [][]byte {
	chunks := make([][]byte, 0, len(calls))
	for i, call := range calls {
		raw, err := marshalSSE(chatChunk{
			ID: id, Object: "chat.completion.chunk", Model: model,
			Choices: []chatChoice{{
				Index: 0,
				Delta: chatDelta{ToolCalls: []chatToolDelta{{
					Index: i, ID: call.ID, Type: "function",
					Function: chatFunctionDelta{Name: call.Function.Name, Arguments: call.Function.Arguments},
				}}},
			}},
		})
		if err == nil {
			chunks = append(chunks, raw)
		}
	}
	return chunks
}

func buildTextChunks(id, model, text string) [][]byte {
	if text == "" {
		return nil
	}
	raw, err := marshalSSE(chatChunk{
		ID: id, Object: "chat.completion.chunk", Model: model,
		Choices: []chatChoice{{Index: 0, Delta: chatDelta{Content: text}}},
	})
	if err != nil {
		return nil
	}
	return [][]byte{raw}
}

func rewriteFinish(raw []byte, newReason string) []byte {
	payload := strings.TrimSpace(string(raw))
	if strings.HasPrefix(payload, "data:") {
		payload = strings.TrimSpace(strings.TrimPrefix(payload, "data:"))
	}
	var cc chatChunk
	if json.Unmarshal([]byte(payload), &cc) != nil {
		return raw
	}
	if len(cc.Choices) > 0 && cc.Choices[0].FinishReason != nil {
		cc.Choices[0].FinishReason = &newReason
	}
	out, err := marshalSSE(cc)
	if err != nil {
		return raw
	}
	return out
}
