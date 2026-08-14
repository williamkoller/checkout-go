package service

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/williamkoller/checkout-go/internal/models"
	"github.com/williamkoller/checkout-go/internal/repository"
	"github.com/williamkoller/checkout-go/internal/saga"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestService(t *testing.T) (*CheckoutService, *gorm.DB) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(&models.Checkout{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	repo := repository.NewCheckoutRepository(db)
	coordinator := saga.NewSagaCoordinator()
	service := NewCheckoutService(repo, redisClient, coordinator)

	return service, db
}

func TestProcessCheckoutWithMultipleItemsCreatesSingleRow(t *testing.T) {
	service, db := setupTestService(t)

	req := models.ProcessCheckoutRequest{
		UserID: "user-1",
		Items: []models.ItemCheckout{
			{ProductID: "prod-001", Quantity: 2, Price: 10.0, Stock: 10},
			{ProductID: "prod-002", Quantity: 1, Price: 5.0, Stock: 10},
		},
		IdenpotencyKey: "idem-abc",
	}

	resp, err := service.ProcessCheckout(context.Background(), req)
	if err != nil {
		t.Fatalf("ProcessCheckout returned error: %v", err)
	}

	var count int64
	if err := db.Model(&models.Checkout{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}

	if resp.Total != 25.0 {
		t.Fatalf("expected total 25.0, got %v", resp.Total)
	}
}

func TestProcessCheckoutWithCorruptedCacheFallsBackToDB(t *testing.T) {
	service, _ := setupTestService(t)

	req := models.ProcessCheckoutRequest{
		UserID: "user-1",
		Items: []models.ItemCheckout{
			{ProductID: "prod-001", Quantity: 2, Price: 10.0, Stock: 10},
		},
		IdenpotencyKey: "idem-cached",
	}

	if _, err := service.ProcessCheckout(context.Background(), req); err != nil {
		t.Fatalf("first ProcessCheckout returned error: %v", err)
	}

	if err := service.redisClient.Set(context.Background(), "idempotency:idem-cached", "not-valid-json", 0).Err(); err != nil {
		t.Fatalf("failed to corrupt cache: %v", err)
	}

	resp, err := service.ProcessCheckout(context.Background(), req)
	if err != nil {
		t.Fatalf("ProcessCheckout returned error: %v", err)
	}

	if resp.ID == "" || resp.Status != "processed" {
		t.Fatalf("expected valid response from DB fallback, got %+v", resp)
	}
}

func TestProcessCheckoutIdempotent(t *testing.T) {
	service, db := setupTestService(t)

	req := models.ProcessCheckoutRequest{
		UserID: "user-1",
		Items: []models.ItemCheckout{
			{ProductID: "prod-001", Quantity: 2, Price: 10.0, Stock: 10},
		},
		IdenpotencyKey: "idem-abc",
	}

	first, err := service.ProcessCheckout(context.Background(), req)
	if err != nil {
		t.Fatalf("first ProcessCheckout returned error: %v", err)
	}

	second, err := service.ProcessCheckout(context.Background(), req)
	if err != nil {
		t.Fatalf("second ProcessCheckout returned error: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected same ID on retry, got %s and %s", first.ID, second.ID)
	}

	var count int64
	if err := db.Model(&models.Checkout{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected 1 row after retry, got %d", count)
	}
}
