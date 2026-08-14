package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/williamkoller/checkout-go/internal/models"
	"github.com/williamkoller/checkout-go/internal/repository"
)

type CheckoutService struct {
	repo        *repository.CheckoutRepository
	redisClient *redis.Client
}

func NewCheckoutService(repo *repository.CheckoutRepository, redisClient *redis.Client) *CheckoutService {
	return &CheckoutService{repo: repo, redisClient: redisClient}
}

func (s *CheckoutService) ProcessCheckout(ctx context.Context, req models.ProcessCheckoutRequest) (*models.ProcessCheckoutResponse, error) {
	cacheKey := fmt.Sprintf("idempotency:%s", req.IdenpotencyKey)
	cachedResult, err := s.redisClient.Get(ctx, cacheKey).Result()

	if err == nil {
		var response models.ProcessCheckoutResponse
		if err := json.Unmarshal([]byte(cachedResult), &response); err == nil && response.ID != "" {
			return &response, nil
		}
	}

	existingCheckout, err := s.repo.FindByIdempotencyKey(req.IdenpotencyKey)
	if err == nil && existingCheckout != nil {
		response := &models.ProcessCheckoutResponse{
			ID:             existingCheckout.IDExternal,
			Status:         existingCheckout.Status,
			Total:          s.calculateTotal(existingCheckout),
			ProcessedIn:    existingCheckout.CreatedAt,
			IdenpotencyKey: existingCheckout.IdempotencyKey,
		}

		s.cacheResult(ctx, cacheKey, response)

		return response, nil
	}

	var total float64
	checkoutID := uuid.New().String()

	for _, item := range req.Items {
		total += item.Price * float64(item.Quantity)
	}

	checkout := &models.Checkout{
		IDExternal:     checkoutID,
		UserID:         req.UserID,
		Items:          req.Items,
		Status:         "processed",
		IdempotencyKey: req.IdenpotencyKey,
	}

	if err := s.repo.Create(checkout); err != nil {
		return nil, fmt.Errorf("error in processed item: %v", err)
	}

	response := &models.ProcessCheckoutResponse{
		ID:             checkoutID,
		Status:         "processed",
		Total:          total,
		ProcessedIn:    time.Now(),
		IdenpotencyKey: req.IdenpotencyKey,
	}

	s.cacheResult(ctx, cacheKey, response)

	return response, nil
}

func (s *CheckoutService) calculateTotal(checkout *models.Checkout) float64 {
	var total float64

	for _, item := range checkout.Items {
		total += item.Price * float64(item.Quantity)
	}

	return total
}

func (s *CheckoutService) cacheResult(ctx context.Context, key string, response *models.ProcessCheckoutResponse) {
	data, err := json.Marshal(response)
	if err == nil {
		s.redisClient.Set(ctx, key, string(data), 24*time.Hour)
	}
}

func (s *CheckoutService) GetCheckout(id string) (*models.Checkout, error) {
	return s.repo.FindByID(id)
}
