package repository

import (
	"github.com/williamkoller/checkout-go/internal/models"
	"gorm.io/gorm"
)

type CheckoutRepository struct {
	db *gorm.DB
}

func NewCheckoutRepository(db *gorm.DB) *CheckoutRepository {
	return &CheckoutRepository{db: db}
}

func (r *CheckoutRepository) Create(checkout *models.Checkout) error {
	return r.db.Create(checkout).Error
}

func (r *CheckoutRepository) FindByIdempotencyKey(key string) (*models.Checkout, error) {
	var checkout models.Checkout

	err := r.db.Where("idempotency_key = ?", key).First(&checkout).Error
	if err != nil {
		return nil, err
	}

	return &checkout, nil
}

func (r *CheckoutRepository) UpdateStatus(id string, status string) error {
	return r.db.Model(&models.Checkout{}).Where("id_external = ?", id).Update("status", status).Error
}

func (r *CheckoutRepository) FindByID(id string) (*models.Checkout, error) {
	var checkout models.Checkout
	err := r.db.Where("id_external = ?", id).First(&checkout).Error
	if err != nil {
		return nil, err
	}

	return &checkout, nil

}
