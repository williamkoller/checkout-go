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
	"github.com/williamkoller/checkout-go/internal/saga"
)

type CheckoutService struct {
	repo        *repository.CheckoutRepository
	redisClient *redis.Client
	coordinator *saga.SagaCoordinator
}

func NewCheckoutService(repo *repository.CheckoutRepository, redisClient *redis.Client, coordinator *saga.SagaCoordinator) *CheckoutService {
	return &CheckoutService{repo: repo, redisClient: redisClient, coordinator: coordinator}
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
			SagaID: existingCheckout.SagaID,
		}

		s.cacheResult(ctx, cacheKey, response)

		return response, nil
	}

	checkoutID := uuid.New().String()
	sagaID := uuid.New().String()

	sagaData := &saga.SagaData{
		SagaID: sagaID,
		CheckoutID: checkoutID,
		UserID: req.UserID,
		Items: req.Items,
		IdempotencyKey: req.IdenpotencyKey,
		Results: make(map[string]interface{}),
		Errors: []string{},
	}

	steps := []saga.Step{
		&saga.StepValidateStock{},
		&saga.StepProcessPayment{},
		&saga.StepSaveCheckout{
			CheckoutRepository: s.repo,
		},
	}

	if err := s.coordinator.ExecuteSaga(ctx, steps, sagaData); err != nil {
		return nil, fmt.Errorf("error in processed item: %v", err)
	}

	total := 0.0

	if value, ok := sagaData.Results["amount_paid"]; ok {
		total = value.(float64)
	}

	response := &models.ProcessCheckoutResponse{
		ID:             checkoutID,
		Status:         "processed",
		Total:          total,
		ProcessedIn:    time.Now(),
		IdenpotencyKey: req.IdenpotencyKey,
		SagaID: sagaID,
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

func (s *CheckoutService) GetStatusSaga(sagaID string) *models.SagaStatus {
	return s.coordinator.GetSagaStatus(sagaID)
}