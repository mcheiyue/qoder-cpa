package main

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport"
)

var errIncompleteChatStream = errors.New("qoder executor: stream ended without [DONE]")

type aggregateChunk struct {
	ID      string            `json:"id"`
	Model   string            `json:"model"`
	Choices []aggregateChoice `json:"choices"`
	Usage   *aggregateUsage   `json:"usage"`
}

type aggregateChoice struct {
	Delta struct {
		Content          string               `json:"content"`
		ReasoningContent string               `json:"reasoning_content"`
		ToolCalls        []aggregateToolDelta `json:"tool_calls"`
	} `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

type aggregateToolDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type aggregateUsage struct {
	PromptTokens            int  `json:"prompt_tokens"`
	CompletionTokens        int  `json:"completion_tokens"`
	TotalTokens             int  `json:"total_tokens"`
	Estimated               bool `json:"estimated,omitempty"`
	CompletionTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

type aggregateResult struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role             string              `json:"role"`
			Content          string              `json:"content,omitempty"`
			ReasoningContent string              `json:"reasoning_content,omitempty"`
			ToolCalls        []aggregateToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *aggregateUsage `json:"usage,omitempty"`
}

type aggregateToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func aggregateChat(handle qodertransport.StreamHandle, inputTokens int) ([]byte, error) {
	result := aggregateResult{Object: "chat.completion"}
	result.Choices = append(result.Choices, struct {
		Index   int `json:"index"`
		Message struct {
			Role             string              `json:"role"`
			Content          string              `json:"content,omitempty"`
			ReasoningContent string              `json:"reasoning_content,omitempty"`
			ToolCalls        []aggregateToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	}{})
	result.Choices[0].Message.Role = "assistant"
	tools := map[int]*aggregateToolCall{}
	done := false
	for !done {
		chunk, err := handle.ReadChunk()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		payload := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(chunk)), "data:"))
		if payload == "[DONE]" {
			done = true
			break
		}
		var event aggregateChunk
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return nil, err
		}
		applyAggregateChunk(&result, tools, event)
	}
	if !done {
		return nil, errIncompleteChatStream
	}
	for index := 0; index < len(tools); index++ {
		if tool := tools[index]; tool != nil {
			result.Choices[0].Message.ToolCalls = append(result.Choices[0].Message.ToolCalls, *tool)
		}
	}
	if result.Usage == nil {
		outputTokens := estimatePayloadTokens([]byte(result.Choices[0].Message.Content))
		outputTokens += estimatePayloadTokens([]byte(result.Choices[0].Message.ReasoningContent))
		for _, tool := range result.Choices[0].Message.ToolCalls {
			outputTokens += estimatePayloadTokens([]byte(tool.Function.Name))
			outputTokens += estimatePayloadTokens([]byte(tool.Function.Arguments))
		}
		result.Usage = &aggregateUsage{
			PromptTokens:     inputTokens,
			CompletionTokens: outputTokens,
			TotalTokens:      inputTokens + outputTokens,
			Estimated:        true,
		}
	}
	return json.Marshal(result)
}

func applyAggregateChunk(result *aggregateResult, tools map[int]*aggregateToolCall, event aggregateChunk) {
	if result.ID == "" {
		result.ID, result.Model = event.ID, event.Model
	}
	if event.Usage != nil {
		result.Usage = event.Usage
	}
	for _, choice := range event.Choices {
		result.Choices[0].Message.Content += choice.Delta.Content
		result.Choices[0].Message.ReasoningContent += choice.Delta.ReasoningContent
		if choice.FinishReason != nil {
			result.Choices[0].FinishReason = *choice.FinishReason
		}
		for _, delta := range choice.Delta.ToolCalls {
			tool := tools[delta.Index]
			if tool == nil {
				tool = &aggregateToolCall{ID: delta.ID, Type: "function"}
				tools[delta.Index] = tool
			}
			if delta.ID != "" {
				tool.ID = delta.ID
			}
			if delta.Function.Name != "" {
				tool.Function.Name = delta.Function.Name
			}
			tool.Function.Arguments += delta.Function.Arguments
		}
	}
}
