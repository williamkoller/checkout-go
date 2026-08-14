package repository

import (
	"testing"

	"github.com/williamkoller/checkout-go/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(&models.Checkout{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func TestFindByIdempotencyKey(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCheckoutRepository(db)

	checkout := &models.Checkout{
		IDExternal:     "ext-123",
		UserID:         "user-1",
		Status:         "processed",
		IdempotencyKey: "idem-key-123",
	}

	if err := repo.Create(checkout); err != nil {
		t.Fatalf("failed to create checkout: %v", err)
	}

	found, err := repo.FindByIdempotencyKey("idem-key-123")
	if err != nil {
		t.Fatalf("FindByIdempotencyKey returned error: %v", err)
	}

	if found == nil || found.IDExternal != "ext-123" {
		t.Fatalf("expected checkout with IDExternal ext-123, got %+v", found)
	}
}
