package ai

import (
	"context"
	"encoding/json"
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

func TestCleanJSONStripsFences(t *testing.T) {
	raw := "```json\n{\"script\":\"dir\",\"is_safe\":true}\n```"
	resp, err := ParseAgentJsonOutput(raw)
	if err != nil {
		t.Fatalf("ParseAgentJsonOutput failed on fenced JSON: %v", err)
	}
	if resp.Script != "dir" || !resp.IsSafe {
		t.Fatalf("unexpected parsed response: %+v", resp)
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

func TestDiscoveryPlanRequestsStructuredOutput(t *testing.T) {
	var received ChatCompletionsRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"questions\":[]}"}}]}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{OpenAIAPIKey: "test-key", OpenAIBaseURL: server.URL, OpenAIModel: "test-model"})
	if err != nil {
		t.Fatal(err)
	}

	plan, err := client.GetDiscoveryPlan([]Message{{Role: "user", Content: "test"}})
	if err != nil {
		t.Fatalf("GetDiscoveryPlan failed: %v", err)
	}
	if len(plan.Questions) != 0 {
		t.Fatalf("expected an empty plan, got %+v", plan.Questions)
	}
	if received.ResponseFormat == nil || received.ResponseFormat.Type != "json_schema" {
		t.Fatalf("expected a json_schema response format, got %+v", received.ResponseFormat)
	}
	if received.ResponseFormat.JSONSchema == nil || received.ResponseFormat.JSONSchema.Name != "discovery_plan" {
		t.Fatalf("expected the discovery_plan schema, got %+v", received.ResponseFormat.JSONSchema)
	}
}

func TestDiscoveryActionParsesNativeToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"list_dir","arguments":"{\"path\":\"C:\\\\tmp\"}"}}]}}]}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{OpenAIAPIKey: "test-key", OpenAIBaseURL: server.URL, OpenAIModel: "test-model"})
	if err != nil {
		t.Fatal(err)
	}

	action, err := client.GetDiscoveryAction("Where is it?", "no facts")
	if err != nil {
		t.Fatalf("GetDiscoveryAction failed: %v", err)
	}
	if action.ToolCall == nil || action.ToolCall.Name != "list_dir" {
		t.Fatalf("expected a native list_dir tool call, got %+v", action)
	}
	if path, _ := action.ToolCall.Args["path"].(string); path != `C:\tmp` {
		t.Fatalf("unexpected tool argument: %+v", action.ToolCall.Args)
	}
}

func TestDiscoveryActionRetriesWithoutUnsupportedTools(t *testing.T) {
	var calls atomic.Int32
	var second ChatCompletionsRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body ChatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"tools are not supported"}`))
			return
		}
		second = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"tool_call\":null,\"answer\":\"answered\",\"done\":true}"}}]}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{OpenAIAPIKey: "test-key", OpenAIBaseURL: server.URL, OpenAIModel: "test-model"})
	if err != nil {
		t.Fatal(err)
	}

	action, err := client.GetDiscoveryAction("Where is it?", "no facts")
	if err != nil {
		t.Fatalf("GetDiscoveryAction should fall back: %v", err)
	}
	if action.Answer != "answered" {
		t.Fatalf("expected the JSON fallback answer, got %+v", action)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected two requests, got %d", calls.Load())
	}
	if len(second.Tools) != 0 {
		t.Fatalf("expected tools to be removed on fallback, got %+v", second.Tools)
	}
}

func TestMockProviderCoversEveryProviderMethod(t *testing.T) {
	provider := NewMockProvider()

	plan, err := provider.GetDiscoveryPlan([]Message{{Role: "user", Content: "test"}})
	if err != nil || plan == nil {
		t.Fatalf("mock discovery plan failed: %v", err)
	}

	action, err := provider.GetDiscoveryAction("question", "state")
	if err != nil || action == nil || !action.Done {
		t.Fatalf("mock discovery action failed: %+v %v", action, err)
	}

	final, err := provider.GetFinalScript([]Message{{Role: "user", Content: "list files"}})
	if err != nil || final == nil || !final.IsSafe || final.Script == "" {
		t.Fatalf("mock final script failed: %+v %v", final, err)
	}
}

func TestClientUseMockNeverReachesTheNetwork(t *testing.T) {
	client, err := NewClient(Config{
		UseMock:       true,
		OpenAIAPIKey:  "unused",
		OpenAIBaseURL: "http://127.0.0.1:1",
		OpenAIModel:   "unused",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.GetDiscoveryPlan([]Message{{Role: "user", Content: "test"}}); err != nil {
		t.Fatalf("mock discovery plan failed: %v", err)
	}
	if _, err := client.GetDiscoveryAction("question", "state"); err != nil {
		t.Fatalf("mock discovery action failed: %v", err)
	}
	if _, err := client.GetFinalScript([]Message{{Role: "user", Content: "list files"}}); err != nil {
		t.Fatalf("mock final script failed: %v", err)
	}
}
