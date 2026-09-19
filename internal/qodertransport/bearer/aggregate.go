package bearer

import (
	"context"
	"errors"
	"io"
	"sort"
)

// Aggregate consumes the same stream primitive as Stream and returns a typed ChatCompletion.
// It does not make a second HTTP request.
func Aggregate(tr *Transport, ctx context.Context, req StreamRequest) (*ChatCompletion, error) {
	resp, err := tr.Stream(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Cancel()
	return collectStream(resp)
}

func collectStream(resp *StreamResponse) (*ChatCompletion, error) {
	cc := &ChatCompletion{
		Object: "chat.completion",
		Choices: []Choice{
			{Index: 0, Message: Message{Role: "assistant"}},
		},
	}
	toolCalls := make(map[int]*toolAccumulator)

	for {
		evt, parseErr := resp.Parser.ParseContext(resp.Context)
		if parseErr != nil {
			if parseErr == io.EOF {
				break
			}
			return nil, parseErr
		}
		switch evt.Type {
		case SSETextDelta:
			if evt.TextDelta != nil {
				cc.Choices[0].Message.Content += evt.TextDelta.Content
			}
		case SSEReasoningDelta:
			if evt.ReasoningDelta != nil {
				cc.Choices[0].Message.Reasoning += evt.ReasoningDelta.Content
			}
		case SSEToolDelta:
			if evt.ToolDelta != nil {
				td := evt.ToolDelta
				acc, ok := toolCalls[td.Index]
				if !ok {
					acc = &toolAccumulator{Index: td.Index}
					toolCalls[td.Index] = acc
				}
				if td.ID != "" {
					acc.ID = td.ID
				}
				if td.Name != "" {
					acc.Name = td.Name
				}
				acc.Arguments += td.Arguments
			}
		case SSEUsage:
			if evt.Usage != nil {
				cc.Usage = &Usage{
					PromptTokens:     evt.Usage.PromptTokens,
					CompletionTokens: evt.Usage.CompletionTokens,
					TotalTokens:      evt.Usage.TotalTokens,
					ReasoningTokens:  evt.Usage.ReasoningTokens,
				}
			}
		case SSEError:
			if evt.StreamError != nil {
				return nil, errors.New("bearer: stream error " + itoa(evt.StreamError.Code))
			}
		}
		// Capture id/model from first chunk that has them.
		if cc.ID == "" && evt.rawID != "" {
			cc.ID = evt.rawID
		}
		if cc.Model == "" && evt.rawModel != "" {
			cc.Model = evt.rawModel
		}
		if cc.Created == 0 && evt.rawCreated != 0 {
			cc.Created = evt.rawCreated
		}
		if evt.Type == SSETerminal {
			if fr, ok := evt.rawFinishReason(); ok {
				cc.Choices[0].FinishReason = fr
			}
		}
	}

	// Convert accumulated tool calls.
	if len(toolCalls) > 0 {
		indices := make([]int, 0, len(toolCalls))
		for idx := range toolCalls {
			indices = append(indices, idx)
		}
		sort.Ints(indices)
		calls := make([]ToolCall, 0, len(indices))
		for _, idx := range indices {
			acc := toolCalls[idx]
			calls = append(calls, ToolCall{
				ID:   acc.ID,
				Type: "function",
				Function: FunctionRef{
					Name:      acc.Name,
					Arguments: acc.Arguments,
				},
			})
		}
		cc.Choices[0].Message.ToolCalls = calls
	}

	// Clean up empty reasoning.
	if cc.Choices[0].Message.Reasoning == "" {
		cc.Choices[0].Message.Reasoning = ""
	}

	return cc, nil
}

type toolAccumulator struct {
	Index     int
	ID        string
	Name      string
	Arguments string
}
