package saga

import (
	"context"
	"fmt"
	"time"

	"github.com/williamkoller/checkout-go/internal/models"
)

type Step interface {
	Execute(ctx context.Context, data *SagaData) error
	Compensate(ctx context.Context, data *SagaData) error
	GetName() string
}

type SagaData struct {
	SagaID         string
	CheckoutID     string
	UserID         string
	Items          []models.ItemCheckout
	IdempotencyKey string
	CurrentStep    int
	Results        map[string]interface{}
	Errors         []string
}

type StepValidateStock struct{}

func (s *StepValidateStock) GetName() string {
	return "validate_stock"
}

func (s *StepValidateStock) Execute(ctx context.Context, data *SagaData) error {
	fmt.Printf("[Saga %s] Execute step: %s\n", data.SagaID, s.GetName())

	for _, item := range data.Items {
		if item.Stock < item.Quantity {
			return fmt.Errorf("Insufficient stock for product %s. Available: %d, Requested: %d", item.ProductID, item.Stock, item.Quantity)
		}

		time.Sleep(50 * time.Millisecond)
	}

	data.Results["srock_validated"] = true
	return nil
}

func (s *StepValidateStock) Compensate(ctx context.Context, data *SagaData) error {
	fmt.Printf("[Saga %s] Compensate step: %s", data.SagaID, s.GetName())
	data.Results["released_stock"] = true
	return nil
}

type StepProcessPayment struct{}

func (s *StepProcessPayment) GetName() string {
	return "process_payment"
}

func (s *StepProcessPayment) Execute(ctx context.Context, data *SagaData) error {
	fmt.Printf("[Saga %s] Execute step: %s\n", data.SagaID, s.GetName())
	var total float64

	for _, item := range data.Items {
		total += item.Price * float64(item.Quantity)
	}

	time.Sleep(100 * time.Millisecond)

	// if time.Now().Unix()%10 == 0 {
	//     return fmt.Errorf("falha no processamento do pagamento")
	// }

	data.Results["payment_id"] = fmt.Sprintf("pay_%s", data.SagaID[:8])
	data.Results["amount_paid"] = total
	data.Results["payment_status"] = "approved"
	return nil
}

func (s *StepProcessPayment) Compensate(ctx context.Context, data *SagaData) error {
	fmt.Printf("[Saga %s] Compensate step: %s", data.SagaID, s.GetName())

	if payID, ok := data.Results["payment_id"]; ok {
		fmt.Printf("[Saga %s] Reversing payment: %v\n", data.SagaID, payID)
		data.Results["reverse_status"] = "processed"
	}

	return nil
}

type StepCreateOrder struct{}

func (s *StepCreateOrder) GetName() string {
	return "create_order"
}

func (s *StepCreateOrder) Execute(ctx context.Context, data *SagaData) error {
	fmt.Printf("[Saga %s] Execute step: %s\n", data.SagaID, s.GetName())
	time.Sleep(50 * time.Millisecond)

	data.Results["order_id"] = fmt.Sprintf("ord_%s", data.SagaID[:8])
	data.Results["order_status"] = "created"
	return nil
}

func (s *StepCreateOrder) Compensate(ctx context.Context, data *SagaData) error {
	fmt.Printf("[Saga %s] Compensate step: %s", data.SagaID, s.GetName())

	if orderID, ok := data.Results["order_id"]; ok {
		fmt.Printf("[Saga %s] Cancelled payment: %v\n", data.SagaID, orderID)
		data.Results["order_status"] = "cancelled"
	}

	return nil
}

type StepSaveCheckout struct {
	CheckoutRepository interface {
		Create(checkout *models.Checkout) error
	}
}

func (s *StepSaveCheckout) GetName() string {
	return "save_checkout"
}

func (s *StepSaveCheckout) Execute(ctx context.Context, data *SagaData) error {
	fmt.Printf("[Saga %s] Execute step: %s\n", data.SagaID, s.GetName())

	checkout := &models.Checkout{
		IDExternal:     data.CheckoutID,
		UserID:         data.UserID,
		Items:          data.Items,
		Status:         "processed",
		IdempotencyKey: data.IdempotencyKey,
		SagaID:         data.SagaID,
		Step:           data.CurrentStep,
	}

	if err := s.CheckoutRepository.Create(checkout); err != nil {
		return fmt.Errorf("error in saved checkout: %v", err)
	}

	data.Results["checkout_saved"] = true
	return nil
}

func (s *StepSaveCheckout) Compensate(ctx context.Context, data *SagaData) error {
	fmt.Printf("[Saga %s] Compensate step: %s", data.SagaID, s.GetName())

	data.Results["checkout_removed"] = true
	return nil
}
