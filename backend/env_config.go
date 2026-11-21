package main

import (
	"bufio"
	"os"
	"strings"
)

type AppConfig struct {
	Port               string
	MongoURI           string
	MongoDB            string
	LlmProvider        string
	LlmEndpoint        string
	LlmModel           string
	HfToken            string
	AllowLiveExecution bool
	LiveAllowedHosts   []string
	AllowedOrigins     []string
	TrustedProxies     []string
	AuthEnabled        bool
	AuthSecret         string
	SeedInstructorUser string
	SeedInstructorPass string
	SeedTraineeUser    string
	SeedTraineePass    string
}

var Config AppConfig

func LoadEnvConfig() {
	readDotEnvFile(".env")
	readDotEnvFile("../.env")
	liveFlag := strings.ToLower(getEnvOrDefault("ALLOW_LIVE_EXECUTION", "false"))
	authFlag := strings.ToLower(getEnvOrDefault("AUTH_ENABLED", "false"))
	Config = AppConfig{
		Port:               getEnvOrDefault("PORT", "8080"),
		MongoURI:           getEnvOrDefault("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:            getEnvOrDefault("MONGO_DB", "soc_platform"),
		LlmProvider:        getEnvOrDefault("LLM_PROVIDER", "ollama"),
		LlmEndpoint:        getEnvOrDefault("LLM_ENDPOINT", "http://localhost:11434"),
		LlmModel:           getEnvOrDefault("LLM_MODEL", "qwen2.5:3b"),
		HfToken:            getEnvOrDefault("HF_TOKEN", ""),
		AllowLiveExecution: liveFlag == "true" || liveFlag == "1" || liveFlag == "yes",
		LiveAllowedHosts:   parseCSVEnv("LIVE_ALLOWED_HOSTS", ""),
		AllowedOrigins:     parseCSVEnv("ALLOWED_ORIGINS", "http://localhost:5173"),
		TrustedProxies:     parseCSVEnv("TRUSTED_PROXIES", ""),
		AuthEnabled:        authFlag == "true" || authFlag == "1" || authFlag == "yes",
		AuthSecret:         getEnvOrDefault("AUTH_SECRET", ""),
		SeedInstructorUser: getEnvOrDefault("SEED_INSTRUCTOR_USER", "instructor"),
		SeedInstructorPass: getEnvOrDefault("SEED_INSTRUCTOR_PASS", "instructor123"),
		SeedTraineeUser:    getEnvOrDefault("SEED_TRAINEE_USER", "trainee"),
		SeedTraineePass:    getEnvOrDefault("SEED_TRAINEE_PASS", "trainee123"),
	}
}

func parseCSVEnv(key, fallback string) []string {
	raw := getEnvOrDefault(key, fallback)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.ToLower(strings.TrimSpace(p)); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func readDotEnvFile(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if os.Getenv(key) == "" {
				_ = os.Setenv(key, val)
			}
		}
	}
}

func getEnvOrDefault(key, fallback string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return fallback
}
