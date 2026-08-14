package models

import (
	"time"

	"gorm.io/gorm"
)

type Checkout struct {
	ID             uint          `gorm:"primaryKey" josn:"id"`
	IDExternal     string        `gorm:"uniqueIndex;not null" json:"id_external"`
	UserID         string        `gorm:"not null" json:"user_id"`
	Items          []ItemCheckout `gorm:"serializer:json" json:"items"`
	Status         string        `gorm:"default:'pending'" json:"status"`
	IdempotencyKey string        `gorm:"uniqueIndex;not null" json:"idempotency_key"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

type ItemCheckout struct {
	ProductId string `json:"product_id" binding:"required"`
	Quantity int `json:"quantity" binding:"required,min=1"`
	Price float64 `json:"price" binding:"required,min=0"`
}

type ProcessCheckoutRequest struct {
	UserID string `json:"user_id" binding:"required"`
	Items []ItemCheckout `json:"items" bonding:"required,min=1"`
	IdenpotencyKey string `json:"idempotency_key"`
}

type ProcessCheckoutResponse struct {
	ID string `json:"id"`
	Status string `json:"status"`
	Total float64 `json:"total"`
	ProcessedIn time.Time `json:"processe_in"`
	IdenpotencyKey string `json:"idempotency_key"`
}