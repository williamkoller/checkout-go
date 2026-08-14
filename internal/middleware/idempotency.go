package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func IdempotencyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == "POST" || c.Request.Method == "PUT" || c.Request.Method == "PATCH" {
			idempotencyKey := c.GetHeader("Idempotency-Key")

			if idempotencyKey == "" {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Idempotency-Key is required",
				})
				c.Abort()
				return
			}

			if !isValidIdempotencyKey(idempotencyKey) {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Idempotency-Key must contain only alphanumeric characters and hyphens.",
				})
				c.Abort()
				return
			}

			c.Set("idempotency_key", idempotencyKey)
		}

		c.Next()
	}
}

func isValidIdempotencyKey(key string) bool {
	for _, char := range key {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-') {
			return false
		}
	}
	return true
}
