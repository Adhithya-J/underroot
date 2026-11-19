// Package config owns application configuration loading, including .env
// discovery, so the command layer does not parse environment state inline.
package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/Adhithya-J/underroot.git/internal/ai"
	"github.com/joho/godotenv"
)

const (
	// EnvAPIKey is the primary API-key variable.
	EnvAPIKey = "OpenAIAPIKey"
	// EnvBaseURL is the primary chat-completions base URL variable.
	EnvBaseURL = "OpenAIBaseURL"
	// EnvModel is the primary model variable.
	EnvModel = "OpenAIModel"
	// EnvUseMock toggles the offline mock provider.
	EnvUseMock = "UNDERROOT_USE_MOCK"

	envAPIKeyAlias  = "OPENAI_API_KEY"
	envBaseURLAlias = "OPENAI_BASE_URL"
	envModelAlias   = "OPENAI_MODEL"
)

// Config is the fully loaded application configuration.
type Config struct {
	AI ai.Config
}

// FromEnv reads configuration values through the supplied lookup function.
// It is deterministic and side-effect free, which keeps it easy to test.
func FromEnv(getenv func(string) string) Config {
	return Config{
		AI: ai.Config{
			OpenAIAPIKey:  firstNonEmpty(getenv(EnvAPIKey), getenv(envAPIKeyAlias)),
			OpenAIBaseURL: firstNonEmpty(getenv(EnvBaseURL), getenv(envBaseURLAlias)),
			OpenAIModel:   firstNonEmpty(getenv(EnvModel), getenv(envModelAlias)),
			UseMock:       parseBool(getenv(EnvUseMock)),
		},
	}
}

// Load reads an optional .env file and validates the resulting configuration.
// A missing .env file is not an error: deployment environments usually inject
// variables directly.
func Load() (Config, error) {
	_ = godotenv.Load()
	cfg := FromEnv(os.Getenv)
	if err := cfg.AI.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseBool(value string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && parsed
}
