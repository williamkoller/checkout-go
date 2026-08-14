package saga

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/williamkoller/checkout-go/internal/models"
)

type SagaCoordinator struct {
	mu             sync.RWMutex
	activeSagas    map[string]*SagaData
	completedSagas map[string]*SagaData
	failedSagas    map[string]*SagaData
}

func NewSagaCoordinator() *SagaCoordinator {
	return &SagaCoordinator{
		activeSagas:    make(map[string]*SagaData),
		completedSagas: make(map[string]*SagaData),
		failedSagas:    make(map[string]*SagaData),
	}
}

func (sc *SagaCoordinator) ExecuteSaga(
	ctx context.Context,
	steps []Step,
	initialData *SagaData,
) error {
	if initialData.SagaID == "" {
		initialData.SagaID = uuid.New().String()
	}

	sc.mu.Lock()
	sc.activeSagas[initialData.SagaID] = initialData
	sc.mu.Unlock()

	defer func() {
		sc.mu.Lock()
		delete(sc.activeSagas, initialData.SagaID)
		sc.mu.Unlock()
	}()

	fmt.Printf("[Saga %s] Initial execute with %d steps\n", initialData.SagaID, len(steps))

	for i, step := range steps {
		initialData.CurrentStep = i

		fmt.Printf("[Saga %s] Execute step %d/%d: %s\n", initialData.SagaID, i+1, len(steps), step.GetName())

		if err := step.Execute(ctx, initialData); err != nil {
			fmt.Printf("[Saga %s] Step %s failed: %v. Initial Compensate..\n", initialData.SagaID, step.GetName(), err)

			initialData.Errors = append(initialData.Errors, err.Error())

			sc.compensateSteps(ctx, steps[:i], initialData)

			sc.mu.Lock()
			sc.failedSagas[initialData.SagaID] = initialData
			sc.mu.Unlock()

			return fmt.Errorf("saga failed in step %s: %v", step.GetName(), err)
		}

		fmt.Printf("[Saga %s] Step %s execute with successfully\n",
			initialData.SagaID, step.GetName())
	}

	// Todos os steps executados com sucesso
	fmt.Printf("[Saga %s] Saga completed with successfully!\n", initialData.SagaID)

	sc.mu.Lock()
	sc.completedSagas[initialData.SagaID] = initialData
	sc.mu.Unlock()

	return nil
}

func (sc *SagaCoordinator) compensateSteps(
	ctx context.Context,
	steps []Step,
	data *SagaData,
) {
	fmt.Printf("[Saga %s] Compensate %d steps...\n", data.SagaID, len(steps))

	// Executar em ordem reversa
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		fmt.Printf("[Saga %s] Compensate step: %s\n", data.SagaID, step.GetName())

		if err := step.Compensate(ctx, data); err != nil {
			fmt.Printf("[Saga %s] ERRO in Compensate step %s: %v\n",
				data.SagaID, step.GetName(), err)

		}
	}
}

func (sc *SagaCoordinator) GetSagaStatus(sagaID string) *models.SagaStatus {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if data, exists := sc.activeSagas[sagaID]; exists {
		return &models.SagaStatus{
			SagaID: sagaID,
			Status: "processing",
			Step:   data.CurrentStep,
		}
	}

	if data, exists := sc.completedSagas[sagaID]; exists {
		return &models.SagaStatus{
			SagaID: sagaID,
			Status: "completed",
			Step:   len(data.Results),
		}
	}

	if data, exists := sc.failedSagas[sagaID]; exists {
		return &models.SagaStatus{
			SagaID: sagaID,
			Status: "failed",
			Step:   data.CurrentStep,
			Error:  data.Errors[0],
		}
	}

	return nil
}
