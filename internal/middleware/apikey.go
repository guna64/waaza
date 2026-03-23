package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func APIKey(required string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if required == "" {
			c.Next()
			return
		}
		got := c.GetHeader("X-API-Key")
		if got == "" || got != required {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"success": false,
				"error":   "unauthorized",
			})
			return
		}
		c.Next()
	}
}

func AdminToken(required string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if required == "" {
			c.Next()
			return
		}
		got := c.GetHeader("Authorization")
		got = strings.TrimSpace(strings.TrimPrefix(got, "Bearer "))
		if got == "" || got != required {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"success": false,
				"error":   "unauthorized",
			})
			return
		}
		c.Next()
	}
}
