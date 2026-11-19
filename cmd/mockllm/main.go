// Command mockllm runs a deterministic, OpenAI-compatible chat-completions
// server. It lets underroot exercise its real HTTP client, structured-output,
// tool-calling, and retry paths without contacting a hosted model.
//
// Usage:
//
//	go run ./cmd/mockllm -addr 127.0.0.1:8080
//
// Then keep OpenAIBaseURL pointing at http://127.0.0.1:8080/ and run underroot.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Adhithya-J/underroot.git/internal/ai"
)

type mockServer struct {
	engine  *ai.ScenarioEngine
	delay   time.Duration
	verbose bool
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	delay := flag.Duration("delay", 0, "artificial latency added to every completion")
	verbose := flag.Bool("verbose", false, "log the request mode for every completion")
	flag.Parse()

	server := &mockServer{
		engine:  ai.NewScenarioEngine(),
		delay:   *delay,
		verbose: *verbose,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/v1/models", server.handleModels)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/chat/completions") {
			server.handleChat(w, r)
			return
		}
		writeError(w, http.StatusNotFound, "unknown endpoint: "+r.URL.Path)
	})

	log.Printf("mock LLM listening on http://%s", *addr)
	log.Printf("OpenAIBaseURL can be http://%s/ or http://%s/v1", *addr, *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("mock LLM stopped: %v", err)
	}
}

func (s *mockServer) handleModels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data": []map[string]any{{
			"id":       "underroot-mock",
			"object":   "model",
			"owned_by": "underroot",
		}},
	})
}

func (s *mockServer) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	var req ai.ChatCompletionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if s.delay > 0 {
		time.Sleep(s.delay)
	}

	content, toolCalls, mode, err := s.complete(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.verbose {
		log.Printf("%s -> %s", r.URL.Path, mode)
	}

	message := map[string]any{
		"role":    "assistant",
		"content": content,
	}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":      "chatcmpl-mock",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   req.Model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       message,
			"finish_reason": "stop",
		}},
		"usage": map[string]int{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	})
}

// complete picks the scenario behavior from the system prompt, mirroring the
// three model roles the client uses.
func (s *mockServer) complete(req ai.ChatCompletionsRequest) (string, []ai.ResponseToolCall, string, error) {
	system, user := splitMessages(req.Messages)

	switch {
	case strings.Contains(system, "filesystem discovery planner"):
		plan, err := s.engine.GetDiscoveryPlan(user)
		if err != nil {
			return "", nil, "discovery_plan", err
		}
		data, err := json.Marshal(plan)
		return string(data), nil, "discovery_plan", err

	case strings.Contains(system, "filesystem discovery assistant"):
		question, state := splitDiscoveryPrompt(user)
		action, err := s.engine.GetDiscoveryAction(question, state)
		if err != nil {
			return "", nil, "discovery_action", err
		}
		if action.ToolCall != nil {
			args, err := json.Marshal(action.ToolCall.Args)
			if err != nil {
				return "", nil, "discovery_action", err
			}
			call := ai.ResponseToolCall{ID: "call_mock_1", Type: "function"}
			call.Function.Name = action.ToolCall.Name
			call.Function.Arguments = string(args)
			return "", []ai.ResponseToolCall{call}, "discovery_action_tool", nil
		}
		data, err := json.Marshal(action)
		return string(data), nil, "discovery_action_answer", err

	default:
		final, err := s.engine.GetFinalScript(user)
		if err != nil {
			return "", nil, "final_script", err
		}
		data, err := json.Marshal(final)
		return string(data), nil, "final_script", err
	}
}

func splitMessages(messages []ai.Message) (system string, user []ai.Message) {
	for _, message := range messages {
		if message.Role == "system" {
			system += message.Content + "\n"
			continue
		}
		user = append(user, message)
	}
	return system, user
}

func splitDiscoveryPrompt(messages []ai.Message) (string, string) {
	if len(messages) == 0 {
		return "", ""
	}
	content := messages[len(messages)-1].Content
	const questionMarker = "QUESTION:\n"
	const stateMarker = "\n\nCONFIRMED STATE:\n"
	questionAt := strings.Index(content, questionMarker)
	stateAt := strings.Index(content, stateMarker)
	if questionAt < 0 || stateAt < 0 || stateAt < questionAt {
		return content, ""
	}
	question := strings.TrimSpace(content[questionAt+len(questionMarker) : stateAt])
	state := strings.TrimSpace(content[stateAt+len(stateMarker):])
	return question, state
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "mock_llm_error",
		},
	})
}
