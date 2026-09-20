package qodertransport

import (
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/bearer"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/cosy"
)

func cosyEventChunk(id, model string, event cosy.SSEEvent, state *handleState) ([]byte, bool, error) {
	switch event.Type {
	case cosy.SSETextDelta:
		if event.TextDelta == nil {
			return nil, false, nil
		}
		return deltaChunk(id, model, chatDelta{Content: event.TextDelta.Content})
	case cosy.SSEReasoningDelta:
		if event.ReasoningDelta == nil {
			return nil, false, nil
		}
		return deltaChunk(id, model, chatDelta{ReasoningContent: event.ReasoningDelta.Content})
	case cosy.SSEToolDelta:
		if event.ToolDelta == nil {
			return nil, false, nil
		}
		tool := event.ToolDelta
		return deltaChunk(id, model, chatDelta{ToolCalls: []chatToolDelta{{
			Index: tool.Index, ID: tool.ID, Type: "function",
			Function: chatFunctionDelta{Name: tool.Name, Arguments: tool.Arguments},
		}}})
	case cosy.SSEUsage:
		if event.Usage == nil {
			return nil, false, nil
		}
		return usageChunk(id, model, event.Usage.PromptTokens, event.Usage.CompletionTokens, event.Usage.TotalTokens, event.Usage.ReasoningTokens)
	case cosy.SSETerminal:
		chunk, err := state.finishChunk()
		return chunk, true, err
	case cosy.SSEError:
		if event.StreamError == nil {
			return nil, false, errUnknownStream
		}
		return nil, false, streamError(event.StreamError.Code)
	default:
		return nil, false, nil
	}
}

func bearerEventChunk(id, model string, event bearer.SSEEvent, state *handleState) ([]byte, bool, error) {
	switch event.Type {
	case bearer.SSETextDelta:
		if event.TextDelta == nil {
			return nil, false, nil
		}
		return deltaChunk(id, model, chatDelta{Content: event.TextDelta.Content})
	case bearer.SSEReasoningDelta:
		if event.ReasoningDelta == nil {
			return nil, false, nil
		}
		return deltaChunk(id, model, chatDelta{ReasoningContent: event.ReasoningDelta.Content})
	case bearer.SSEToolDelta:
		if event.ToolDelta == nil {
			return nil, false, nil
		}
		tool := event.ToolDelta
		return deltaChunk(id, model, chatDelta{ToolCalls: []chatToolDelta{{
			Index: tool.Index, ID: tool.ID, Type: "function",
			Function: chatFunctionDelta{Name: tool.Name, Arguments: tool.Arguments},
		}}})
	case bearer.SSEUsage:
		if event.Usage == nil {
			return nil, false, nil
		}
		return usageChunk(id, model, event.Usage.PromptTokens, event.Usage.CompletionTokens, event.Usage.TotalTokens, event.Usage.ReasoningTokens)
	case bearer.SSETerminal:
		chunk, err := state.finishChunk()
		return chunk, true, err
	case bearer.SSEError:
		if event.StreamError == nil {
			return nil, false, errUnknownStream
		}
		return nil, false, streamError(event.StreamError.Code)
	default:
		return nil, false, nil
	}
}

func deltaChunk(id, model string, delta chatDelta) ([]byte, bool, error) {
	chunk, err := marshalSSE(chatChunk{
		ID: id, Object: "chat.completion.chunk", Model: model,
		Choices: []chatChoice{{Index: 0, Delta: delta}},
	})
	return chunk, true, err
}

func usageChunk(id, model string, prompt, completion, total, reasoning int) ([]byte, bool, error) {
	usage := &chatUsage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: total}
	usage.CompletionTokensDetails.ReasoningTokens = reasoning
	chunk, err := marshalSSE(chatChunk{ID: id, Object: "chat.completion.chunk", Model: model, Choices: []chatChoice{}, Usage: usage})
	return chunk, true, err
}
