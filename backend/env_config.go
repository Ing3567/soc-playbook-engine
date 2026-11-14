package main

import (
	"bufio"
	"os"
	"strings"
)

type AppConfig struct {
	Port        string
	MongoURI    string
	MongoDB     string
	LlmProvider string
	LlmEndpoint string
	LlmModel    string
	HfToken     string
}

var Config AppConfig

func LoadEnvConfig() {
	readDotEnvFile("//.env")
	Config = AppConfig{
		Port:        getEnvOrDefault("PORT", "8080"),
		MongoURI:    getEnvOrDefault("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:     getEnvOrDefault("MONGO_DB", "soc_platform"),
		LlmProvider: getEnvOrDefault("LLM_PROVIDER", "ollama"),
		LlmEndpoint: getEnvOrDefault("LLM_ENDPOINT", "http://localhost:11434"),
		LlmModel:    getEnvOrDefault("LLM_MODEL", "qwen2.5:3b"),
		HfToken:     getEnvOrDefault("HF_TOKEN", ""),
	}
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