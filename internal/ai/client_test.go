package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParseAgentJSONOutputRejectsEmptyResponse(t *testing.T) {
	_, err := ParseAgentJsonOutput(" \n\t ")
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("expected an empty-response error, got %v", err)
	}
}

func TestFinalSystemPromptRequiresSafeFlag(t *testing.T) {
	prompt := FinalSystemPrompt().Content
	for _, required := range []string{"\"is_safe\":true", "All four keys are required", "do not omit it"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("final prompt must require %q", required)
		}
	}
}

func TestChatRetriesTransportFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_ = connection.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{OpenAIAPIKey: "test-key", OpenAIBaseURL: server.URL, OpenAIModel: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Chat([]Message{{Role: "user", Content: "test"}}); err != nil {
		t.Fatalf("Chat should recover from one closed connection: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected two transport attempts, got %d", calls.Load())
	}
}

func TestChatRejectsEmptyCompletionContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"   "}}]}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		OpenAIAPIKey:  "test-key",
		OpenAIBaseURL: server.URL,
		OpenAIModel:   "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.chatWithSystem(context.Background(), FinalSystemPrompt(), []Message{{Role: "user", Content: "test"}})
	if err == nil || !strings.Contains(err.Error(), "empty completion content") {
		t.Fatalf("expected an empty-completion error, got %v", err)
	}
}
