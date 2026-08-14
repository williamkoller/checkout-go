package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/williamkoller/checkout-go/internal/models"
	"github.com/williamkoller/checkout-go/internal/service"
)

type CheckoutHandler struct {
	service *service.CheckoutService
}

func NewCheckoutHandler(service *service.CheckoutService) *CheckoutHandler {
	return &CheckoutHandler{service: service}
}

func (h *CheckoutHandler) ProcessCheckout(c *gin.Context) {
	var req models.ProcessCheckoutRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "data invalid: " + err.Error(),
		})
		return
	}

	idempotencyKey, exists := c.Get("idempotency_key")
	if exists {
		req.IdenpotencyKey = idempotencyKey.(string)
	}

	response, err := h.service.ProcessCheckout(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "error in processing checkout: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *CheckoutHandler) GetCheckout(c *gin.Context) {
	id := c.Param("id")

	checkout, err := h.service.GetCheckout(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "checkout not found",
		})
		return
	}

	c.JSON(http.StatusOK, checkout)
}

func (h *CheckoutHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": "service of checkout is running",
	})
}
