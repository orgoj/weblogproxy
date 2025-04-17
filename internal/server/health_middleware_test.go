package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/orgoj/weblogproxy/internal/iputil"
	"github.com/stretchr/testify/assert"
)

// healthIPMiddlewareTest simulates the health IP filter middleware.
func healthIPMiddlewareTest(allowed []string) gin.HandlerFunc {
	parsed, err := iputil.ParseCIDRs(allowed)
	if err != nil {
		return func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal server error (health config)"})
		}
	}
	return func(c *gin.Context) {
		// X-Forwarded-For override if present
		var ip net.IP
		if xff := c.Request.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if first := strings.TrimSpace(parts[0]); net.ParseIP(first) != nil {
				ip = net.ParseIP(first)
			}
		}
		// Fallback to remote address
		if ip == nil {
			host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
			if err != nil {
				host = c.Request.RemoteAddr
			}
			ip = net.ParseIP(host)
		}
		if ip == nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		if len(parsed) > 0 && !iputil.IsIPInAnyCIDR(ip, parsed) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}

func TestHealthIPMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		allowed      []string
		remoteAddr   string
		xff          string
		expectedCode int
	}{
		{
			name:         "Allowed IP direct",
			allowed:      []string{"192.168.0.0/16"},
			remoteAddr:   "192.168.1.10:1234",
			expectedCode: http.StatusOK,
		},
		{
			name:         "Denied IP direct",
			allowed:      []string{"10.0.0.0/8"},
			remoteAddr:   "192.168.1.10:1234",
			expectedCode: http.StatusForbidden,
		},
		{
			name:         "Allowed via XFF",
			allowed:      []string{"172.16.0.0/12"},
			remoteAddr:   "203.0.113.1:5678",
			xff:          "172.16.5.10",
			expectedCode: http.StatusOK,
		},
		{
			name:         "Invalid IP in request",
			allowed:      []string{"0.0.0.0/0"},
			remoteAddr:   "invalid-ip",
			expectedCode: http.StatusForbidden,
		},
		{
			name:         "Invalid config",
			allowed:      []string{"not-a-cidr"},
			remoteAddr:   "192.168.1.10:1234",
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/health", healthIPMiddlewareTest(tt.allowed), func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/health", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}
