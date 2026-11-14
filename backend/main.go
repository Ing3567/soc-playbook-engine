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

	router := gin.Default()
	handler := NewApiHandler(repo)
	handler.RegisterRoutes(router)

	if err := router.Run(":" + Config.Port); err != nil {
		log.Fatal(err)
	}
}