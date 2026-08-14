package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/williamkoller/checkout-go/internal/handlers"
	"github.com/williamkoller/checkout-go/internal/middleware"
	"github.com/williamkoller/checkout-go/internal/models"
	"github.com/williamkoller/checkout-go/internal/repository"
	"github.com/williamkoller/checkout-go/internal/saga"
	"github.com/williamkoller/checkout-go/internal/service"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	db, err := gorm.Open(sqlite.Open("checkout.db"), &gorm.Config{})
	if err != nil {
		log.Fatal("failed in connected database")
	}

	if err := db.AutoMigrate(&models.Checkout{}); err != nil {
		log.Fatal("failed in migrate database")
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_ADDR"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatal("failed in connected redis: ", err)
	}

	repo := repository.NewCheckoutRepository(db)
	coordinator := saga.NewSagaCoordinator()
	service := service.NewCheckoutService(repo, redisClient, coordinator)
	handler := handlers.NewCheckoutHandler(service)

	router := gin.Default()

	router.Use(middleware.IdempotencyMiddleware())

	api := router.Group("/api/v1")
	{
		api.POST("/checkout/process", handler.ProcessCheckout)
		api.GET("/checkout/:id", handler.GetCheckout)
		api.GET("/saga/:saga_id/status", handler.GetSagaStatus)
		api.GET("/health", handler.HealthCheck)
	}

	port := os.Getenv("PORT")

	if port == "" {
		port = "8082"
	}

	fmt.Printf("server initial in port %s\n", port)

	if err := router.Run(":" + port); err != nil {
		log.Fatal("failed in initial server: ", err)
	}
}
