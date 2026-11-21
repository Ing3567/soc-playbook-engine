package main

import (
	"context"
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	LoadEnvConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo, err := NewPlaybookRepository(ctx, Config.MongoURI, Config.MongoDB)
	if err != nil {
		log.Fatalf("MongoDB error: %v", err)
	}

	toolRepo := NewToolRepository(repo.Database())
	if err := toolRepo.EnsureBuiltinTools(ctx); err != nil {
		log.Fatalf("Tool seeding error: %v", err)
	}

	scenarioRepo := NewScenarioRepository(repo.Database())
	if err := scenarioRepo.EnsureBuiltinScenarios(ctx); err != nil {
		log.Fatalf("Scenario seeding error: %v", err)
	}

	router := gin.Default()
	if err := router.SetTrustedProxies(Config.TrustedProxies); err != nil {
		log.Fatalf("Invalid TRUSTED_PROXIES config: %v", err)
	}
	router.Use(CORSMiddleware(Config.AllowedOrigins))
	handler := NewApiHandler(repo, toolRepo, scenarioRepo)
	handler.RegisterRoutes(router)

	if err := router.Run(":" + Config.Port); err != nil {
		log.Fatal(err)
	}
}
