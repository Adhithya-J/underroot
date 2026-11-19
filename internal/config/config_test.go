package config

import "testing"

func TestFromEnvReadsPrimaryVariables(t *testing.T) {
	values := map[string]string{
		EnvAPIKey:  "primary-key",
		EnvBaseURL: "https://api.example.com/v1",
		EnvModel:   "primary-model",
	}
	cfg := FromEnv(func(key string) string { return values[key] })

	if cfg.AI.OpenAIAPIKey != "primary-key" || cfg.AI.OpenAIBaseURL != "https://api.example.com/v1" || cfg.AI.OpenAIModel != "primary-model" {
		t.Fatalf("unexpected config: %+v", cfg.AI)
	}
	if cfg.AI.UseMock {
		t.Fatal("mock mode should default to false")
	}
}

func TestFromEnvSupportsAliasesAndMock(t *testing.T) {
	values := map[string]string{
		envAPIKeyAlias:  "alias-key",
		envBaseURLAlias: "http://localhost:11434/v1",
		envModelAlias:   "alias-model",
		EnvUseMock:      "true",
	}
	cfg := FromEnv(func(key string) string { return values[key] })

	if cfg.AI.OpenAIAPIKey != "alias-key" || cfg.AI.OpenAIBaseURL != "http://localhost:11434/v1" || cfg.AI.OpenAIModel != "alias-model" {
		t.Fatalf("unexpected alias config: %+v", cfg.AI)
	}
	if !cfg.AI.UseMock {
		t.Fatal("expected mock mode to be enabled")
	}
}

func TestFromEnvPrefersPrimaryOverAlias(t *testing.T) {
	values := map[string]string{
		EnvAPIKey:      "primary-key",
		envAPIKeyAlias: "alias-key",
	}
	cfg := FromEnv(func(key string) string { return values[key] })
	if cfg.AI.OpenAIAPIKey != "primary-key" {
		t.Fatalf("expected the primary key, got %q", cfg.AI.OpenAIAPIKey)
	}
}
