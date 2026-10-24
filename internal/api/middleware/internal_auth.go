package middleware

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// InternalAuthMiddleware protects backend-to-backend endpoints (the worker DEK
// fetch is the first user) with a static bearer token sourced from the
// INTERNAL_API_TOKEN env var. Constant-time compare to defeat timing oracles.
//
// This is a deliberately simple primitive — workers and backend share one
// secret out-of-band (env var in both processes). Task #9 will replace this
// with per-worker JWTs minted at registration time.
//
// If INTERNAL_API_TOKEN is unset, every request is rejected — fail closed.
func (h *Handler) InternalAuthMiddleware() gin.HandlerFunc {
	return internalAuth
}

var (
	internalTokenOnce sync.Once
	internalToken     []byte
)

func loadInternalToken() {
	if v := os.Getenv("INTERNAL_API_TOKEN"); v != "" {
		internalToken = []byte(v)
	}
}

func internalAuth(c *gin.Context) {
	internalTokenOnce.Do(loadInternalToken)
	if len(internalToken) == 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "internal auth not configured"})
		return
	}
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
		return
	}
	provided := []byte(strings.TrimPrefix(header, "Bearer "))
	if subtle.ConstantTimeCompare(provided, internalToken) != 1 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid bearer token"})
		return
	}
	c.Next()
}
