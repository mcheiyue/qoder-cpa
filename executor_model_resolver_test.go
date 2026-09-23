package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport"
)

func TestExecutorPassesAuthScopedModelResolver(t *testing.T) {
	registry := newModelRegistry()
	registry.storeCatalog(
		"qoder-user",
		map[string]string{"qoder/Qwen3.8-Flash": "qfmodel"},
		map[string]ModelMeta{"qoder/Qwen3.8-Flash": {IsReasoning: true, MaxInputTokens: 131072}},
	)
	transport := &fakeChatTransport{handle: &fakeChatHandle{chunks: executorChatChunks()}}
	var resolve qodertransport.ModelResolver
	service := executorService{
		hostCall: func(string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil },
		registry: registry,
		selectorFactory: func(_ *http.Client, _ qoderauth.Credential, modelResolve qodertransport.ModelResolver) (transportSelector, error) {
			resolve = modelResolve
			return fakeSelector{transport: transport}, nil
		},
	}
	if _, err := service.open(context.Background(), executorRequest(t, false)); err != nil {
		t.Fatal(err)
	}
	if resolve == nil {
		t.Fatal("executor did not pass the auth-scoped model resolver")
	}
	resolved := resolve("qoder/Qwen3.8-Flash")
	if resolved.InternalID != "qfmodel" || !resolved.IsReasoning || resolved.MaxInputTokens != 131072 {
		t.Fatalf("resolved model=%+v", resolved)
	}
}
