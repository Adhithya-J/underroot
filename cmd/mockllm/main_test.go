package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Adhithya-J/underroot.git/internal/ai"
)

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

func postChat(t *testing.T, server *mockServer, req ai.ChatCompletionsRequest) chatResponse {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	server.handleChat(recorder, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", recorder.Code, recorder.Body.String())
	}
	var resp chatResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	return resp
}

func TestMockServerReturnsFinalScript(t *testing.T) {
	server := &mockServer{engine: ai.NewScenarioEngine()}
	resp := postChat(t, server, ai.ChatCompletionsRequest{
		Model: "underroot-mock",
		Messages: []ai.Message{
			ai.FinalSystemPrompt(),
			{Role: "user", Content: "TASK:\nlist files in this folder\n\nENVIRONMENT:\nFolder: demo\nWorking Directory: C:\\demo\nParent Directory: C:\\\nVerified Known Folders:\nNone. Use filesystem discovery tools for path questions.\n\nCONFIRMED DISCOVERY FACTS:\nNo additional filesystem facts required."},
		},
	})

	if len(resp.Choices) != 1 {
		t.Fatalf("expected one choice, got %+v", resp)
	}
	if !strings.Contains(resp.Choices[0].Message.Content, "Get-ChildItem") {
		t.Fatalf("expected a listing script, got %q", resp.Choices[0].Message.Content)
	}
}

func TestMockServerReturnsDiscoveryToolCall(t *testing.T) {
	server := &mockServer{engine: ai.NewScenarioEngine()}
	resp := postChat(t, server, ai.ChatCompletionsRequest{
		Model: "underroot-mock",
		Messages: []ai.Message{
			ai.DiscoveryActionPrompt(),
			{Role: "user", Content: "QUESTION:\nWhere is report.pdf? [mock:find:report.pdf]\n\nCONFIRMED STATE:\nWorking Directory: C:\\demo"},
		},
		Tools: ai.DiscoveryTools(),
	})

	calls := resp.Choices[0].Message.ToolCalls
	if len(calls) != 1 || calls[0].Function.Name != "search_entries" {
		t.Fatalf("expected a search_entries tool call, got %+v", calls)
	}
	if !strings.Contains(calls[0].Function.Arguments, `C:\\demo`) {
		t.Fatalf("expected the working directory in arguments, got %s", calls[0].Function.Arguments)
	}
}

func TestMockServerReturnsDiscoveryPlan(t *testing.T) {
	server := &mockServer{engine: ai.NewScenarioEngine()}
	resp := postChat(t, server, ai.ChatCompletionsRequest{
		Model: "underroot-mock",
		Messages: []ai.Message{
			ai.DiscoveryPlanPrompt(),
			{Role: "user", Content: "TASK:\nopen report.pdf\n\nENVIRONMENT:\nFolder: demo\nWorking Directory: C:\\demo\nParent Directory: C:\\\nVerified Known Folders:\nNone. Use filesystem discovery tools for path questions."},
		},
	})

	if !strings.Contains(resp.Choices[0].Message.Content, "mock:find:report.pdf") {
		t.Fatalf("expected a discovery question, got %q", resp.Choices[0].Message.Content)
	}
}
