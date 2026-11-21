package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	LoadEnvConfig()

	if Config.AuthEnabled && strings.TrimSpace(Config.AuthSecret) == "" {
		log.Println("WARNING: AUTH_ENABLED=true without AUTH_SECRET; using ephemeral secret (tokens reset on restart)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo, err := NewPlaybookRepository(ctx, Config.MongoURI, Config.MongoDB)
	if err != nil {
		log.Fatalf("MongoDB error: %v", err)
	}
	if err := repo.EnsureIndexes(ctx); err != nil {
		log.Fatalf("Playbook index setup error: %v", err)
	}

	toolRepo := NewToolRepository(repo.Database())
	if err := toolRepo.EnsureBuiltinTools(ctx); err != nil {
		log.Fatalf("Tool seeding error: %v", err)
	}

	scenarioRepo := NewScenarioRepository(repo.Database())
	if err := scenarioRepo.EnsureBuiltinScenarios(ctx); err != nil {
		log.Fatalf("Scenario seeding error: %v", err)
	}

	userRepo := NewUserRepository(repo.Database())
	if err := userRepo.EnsureIndexesAndSeedUsers(
		ctx,
		Config.SeedInstructorUser,
		Config.SeedInstructorPass,
		Config.SeedTraineeUser,
		Config.SeedTraineePass,
	); err != nil {
		log.Fatalf("User seeding error: %v", err)
	}

	runRepo := NewRunRepository(repo.Database())
	if err := runRepo.EnsureIndexes(ctx); err != nil {
		log.Fatalf("Run index setup error: %v", err)
	}

	router := gin.Default()
	if err := router.SetTrustedProxies(Config.TrustedProxies); err != nil {
		log.Fatalf("Invalid TRUSTED_PROXIES config: %v", err)
	}
	router.Use(CORSMiddleware(Config.AllowedOrigins))
	handler := NewApiHandler(repo, toolRepo, scenarioRepo, userRepo, runRepo)
	handler.RegisterRoutes(router)

	if err := router.Run(":" + Config.Port); err != nil {
		log.Fatal(err)
	}
}
