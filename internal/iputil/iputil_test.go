package iputil

import (
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCIDRs(t *testing.T) {
	tests := []struct {
		name        string
		input       []string
		expectError bool
		expectCount int
	}{
		{
			name:        "Empty input",
			input:       []string{},
			expectError: false,
			expectCount: 0,
		},
		{
			name:        "Nil input",
			input:       nil,
			expectError: false,
			expectCount: 0,
		},
		{
			name:        "Single IPv4 address",
			input:       []string{"192.168.1.1"},
			expectError: false,
			expectCount: 1,
		},
		{
			name:        "Single IPv4 CIDR",
			input:       []string{"192.168.1.0/24"},
			expectError: false,
			expectCount: 1,
		},
		{
			name:        "Single IPv6 address",
			input:       []string{"2001:db8::1"},
			expectError: false,
			expectCount: 1,
		},
		{
			name:        "Single IPv6 CIDR",
			input:       []string{"2001:db8::/32"},
			expectError: false,
			expectCount: 1,
		},
		{
			name:        "Multiple mixed entries",
			input:       []string{"192.168.1.1", "10.0.0.0/8", "2001:db8::1"},
			expectError: false,
			expectCount: 3,
		},
		{
			name:        "Invalid IP format",
			input:       []string{"not-an-ip"},
			expectError: true,
			expectCount: 0,
		},
		{
			name:        "Invalid CIDR format",
			input:       []string{"192.168.1.1/99"},
			expectError: true,
			expectCount: 0,
		},
		{
			name:        "Mixed valid and invalid",
			input:       []string{"192.168.1.1", "invalid"},
			expectError: true,
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseCIDRs(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				if tt.expectCount == 0 {
					assert.Nil(t, result)
				} else {
					require.NotNil(t, result)
					assert.Len(t, result, tt.expectCount)
				}
			}
		})
	}
}

func TestParseCIDRs_SingleIPConversion(t *testing.T) {
	// Test that single IPs are converted to /32 (IPv4) or /128 (IPv6)
	t.Run("IPv4 single IP becomes /32", func(t *testing.T) {
		result, err := ParseCIDRs([]string{"192.168.1.1"})
		require.NoError(t, err)
		require.Len(t, result, 1)

		// Check mask is /32
		ones, bits := result[0].Mask.Size()
		assert.Equal(t, 32, ones)
		assert.Equal(t, 32, bits)
	})

	t.Run("IPv6 single IP becomes /128", func(t *testing.T) {
		result, err := ParseCIDRs([]string{"2001:db8::1"})
		require.NoError(t, err)
		require.Len(t, result, 1)

		// Check mask is /128
		ones, bits := result[0].Mask.Size()
		assert.Equal(t, 128, ones)
		assert.Equal(t, 128, bits)
	})
}

func TestIsIPInAnyCIDR(t *testing.T) {
	// Setup test CIDRs
	cidrs, err := ParseCIDRs([]string{
		"192.168.1.0/24",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"2001:db8::/32",
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{
			name:     "IP in first CIDR",
			ip:       "192.168.1.100",
			expected: true,
		},
		{
			name:     "IP in second CIDR",
			ip:       "10.5.10.20",
			expected: true,
		},
		{
			name:     "IP in third CIDR",
			ip:       "172.16.5.1",
			expected: true,
		},
		{
			name:     "IPv6 in CIDR",
			ip:       "2001:db8::1234",
			expected: true,
		},
		{
			name:     "IP not in any CIDR",
			ip:       "1.2.3.4",
			expected: false,
		},
		{
			name:     "Private IP not in ranges",
			ip:       "192.168.2.1",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip, "Failed to parse IP: %s", tt.ip)

			result := IsIPInAnyCIDR(ip, cidrs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsIPInAnyCIDR_EdgeCases(t *testing.T) {
	cidrs, err := ParseCIDRs([]string{"192.168.1.0/24"})
	require.NoError(t, err)

	t.Run("Nil IP", func(t *testing.T) {
		result := IsIPInAnyCIDR(nil, cidrs)
		assert.False(t, result)
	})

	t.Run("Empty CIDR list", func(t *testing.T) {
		ip := net.ParseIP("192.168.1.1")
		result := IsIPInAnyCIDR(ip, []*net.IPNet{})
		assert.False(t, result)
	})

	t.Run("Nil CIDR list", func(t *testing.T) {
		ip := net.ParseIP("192.168.1.1")
		result := IsIPInAnyCIDR(ip, nil)
		assert.False(t, result)
	})

	t.Run("Both nil", func(t *testing.T) {
		result := IsIPInAnyCIDR(nil, nil)
		assert.False(t, result)
	})
}

func TestGetClientIP(t *testing.T) {
	trustedProxies, err := ParseCIDRs([]string{"10.0.0.1", "10.0.0.2"})
	require.NoError(t, err)

	tests := []struct {
		name            string
		remoteAddr      string
		customHeader    string
		customHeaderVal string
		xForwardedFor   string
		trustedProxies  []*net.IPNet
		expectedIP      string
		description     string
	}{
		{
			name:            "SECURITY: Custom header ignored from untrusted source",
			remoteAddr:      "1.2.3.4:12345",
			customHeader:    "X-Real-IP",
			customHeaderVal: "99.99.99.99",
			trustedProxies:  trustedProxies,
			expectedIP:      "1.2.3.4",
			description:     "Critical: Prevents IP spoofing when request is NOT from trusted proxy",
		},
		{
			name:            "SECURITY: Custom header accepted from trusted proxy",
			remoteAddr:      "10.0.0.1:12345",
			customHeader:    "X-Real-IP",
			customHeaderVal: "99.99.99.99",
			trustedProxies:  trustedProxies,
			expectedIP:      "99.99.99.99",
			description:     "Custom header trusted when request comes from trusted proxy",
		},
		{
			name:            "SECURITY: CF-Connecting-IP from trusted source",
			remoteAddr:      "10.0.0.2:54321",
			customHeader:    "CF-Connecting-IP",
			customHeaderVal: "8.8.8.8",
			trustedProxies:  trustedProxies,
			expectedIP:      "8.8.8.8",
			description:     "Cloudflare header accepted from trusted proxy",
		},
		{
			name:            "SECURITY: CF-Connecting-IP ignored from untrusted",
			remoteAddr:      "1.1.1.1:54321",
			customHeader:    "CF-Connecting-IP",
			customHeaderVal: "8.8.8.8",
			trustedProxies:  trustedProxies,
			expectedIP:      "1.1.1.1",
			description:     "Cloudflare header ignored when not from trusted source",
		},
		{
			name:           "X-Forwarded-For from trusted proxy",
			remoteAddr:     "10.0.0.1:12345",
			xForwardedFor:  "5.5.5.5, 10.0.0.1",
			trustedProxies: trustedProxies,
			expectedIP:     "5.5.5.5",
			description:    "X-Forwarded-For first IP used when from trusted proxy",
		},
		{
			name:           "X-Forwarded-For from untrusted proxy ignored",
			remoteAddr:     "1.2.3.4:12345",
			xForwardedFor:  "5.5.5.5, 1.2.3.4",
			trustedProxies: trustedProxies,
			expectedIP:     "1.2.3.4",
			description:    "X-Forwarded-For ignored when not from trusted proxy",
		},
		{
			name:           "Fallback to RemoteAddr",
			remoteAddr:     "7.7.7.7:12345",
			trustedProxies: trustedProxies,
			expectedIP:     "7.7.7.7",
			description:    "Falls back to RemoteAddr when no headers present",
		},
		{
			name:           "RemoteAddr without port",
			remoteAddr:     "8.8.8.8",
			trustedProxies: trustedProxies,
			expectedIP:     "8.8.8.8",
			description:    "Handles RemoteAddr without port",
		},
		{
			name:            "Custom header with whitespace",
			remoteAddr:      "10.0.0.1:12345",
			customHeader:    "X-Real-IP",
			customHeaderVal: "  9.9.9.9  ",
			trustedProxies:  trustedProxies,
			expectedIP:      "9.9.9.9",
			description:     "Trims whitespace from custom header",
		},
		{
			name:            "Invalid IP in custom header",
			remoteAddr:      "10.0.0.1:12345",
			customHeader:    "X-Real-IP",
			customHeaderVal: "not-an-ip",
			trustedProxies:  trustedProxies,
			expectedIP:      "10.0.0.1",
			description:     "Falls through when custom header contains invalid IP",
		},
		{
			name:            "Empty custom header value",
			remoteAddr:      "10.0.0.1:12345",
			customHeader:    "X-Real-IP",
			customHeaderVal: "",
			trustedProxies:  trustedProxies,
			expectedIP:      "10.0.0.1",
			description:     "Falls through when custom header is empty",
		},
		{
			name:            "Empty trusted proxies",
			remoteAddr:      "1.2.3.4:12345",
			customHeader:    "X-Real-IP",
			customHeaderVal: "99.99.99.99",
			trustedProxies:  []*net.IPNet{},
			expectedIP:      "1.2.3.4",
			description:     "Custom header ignored when no trusted proxies configured",
		},
		{
			name:            "Priority: Custom header over X-Forwarded-For",
			remoteAddr:      "10.0.0.1:12345",
			customHeader:    "X-Real-IP",
			customHeaderVal: "11.11.11.11",
			xForwardedFor:   "22.22.22.22",
			trustedProxies:  trustedProxies,
			expectedIP:      "11.11.11.11",
			description:     "Custom header takes priority over X-Forwarded-For",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/", nil)
			require.NoError(t, err)

			req.RemoteAddr = tt.remoteAddr

			if tt.customHeaderVal != "" && tt.customHeader != "" {
				req.Header.Set(tt.customHeader, tt.customHeaderVal)
			}

			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}

			result := GetClientIP(req, tt.trustedProxies, tt.customHeader)
			assert.Equal(t, tt.expectedIP, result, tt.description)
		})
	}
}

func TestGetClientIP_RealWorldScenarios(t *testing.T) {
	t.Run("Cloudflare setup", func(t *testing.T) {
		// Simulating Cloudflare proxy
		cfProxies, _ := ParseCIDRs([]string{"173.245.48.0/20", "103.21.244.0/22"})

		req, _ := http.NewRequest("GET", "/", nil)
		req.RemoteAddr = "173.245.48.10:443"
		req.Header.Set("CF-Connecting-IP", "203.0.113.42")
		req.Header.Set("X-Forwarded-For", "203.0.113.42, 173.245.48.10")

		ip := GetClientIP(req, cfProxies, "CF-Connecting-IP")
		assert.Equal(t, "203.0.113.42", ip, "Should extract real client IP from Cloudflare")
	})

	t.Run("nginx reverse proxy", func(t *testing.T) {
		// nginx proxy
		nginxProxy, _ := ParseCIDRs([]string{"127.0.0.1"})

		req, _ := http.NewRequest("GET", "/", nil)
		req.RemoteAddr = "127.0.0.1:56789"
		req.Header.Set("X-Real-IP", "198.51.100.23")
		req.Header.Set("X-Forwarded-For", "198.51.100.23")

		ip := GetClientIP(req, nginxProxy, "X-Real-IP")
		assert.Equal(t, "198.51.100.23", ip, "Should extract real client IP from nginx")
	})

	t.Run("Direct connection (no proxy)", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/", nil)
		req.RemoteAddr = "203.0.113.100:45678"

		ip := GetClientIP(req, nil, "")
		assert.Equal(t, "203.0.113.100", ip, "Should use RemoteAddr for direct connections")
	})

	t.Run("Attacker trying to spoof via X-Real-IP", func(t *testing.T) {
		// Attacker sends X-Real-IP header but isn't from trusted proxy
		trustedProxies, _ := ParseCIDRs([]string{"10.0.0.0/8"})

		req, _ := http.NewRequest("GET", "/", nil)
		req.RemoteAddr = "203.0.113.50:12345"  // Attacker's real IP
		req.Header.Set("X-Real-IP", "1.1.1.1") // Trying to spoof as 1.1.1.1

		ip := GetClientIP(req, trustedProxies, "X-Real-IP")
		assert.Equal(t, "203.0.113.50", ip, "SECURITY: Must not accept spoofed header from untrusted source")
	})

	t.Run("Chain of proxies", func(t *testing.T) {
		trustedProxies, _ := ParseCIDRs([]string{"10.0.0.1"})

		req, _ := http.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.1:443"
		req.Header.Set("X-Forwarded-For", "198.51.100.99, 192.0.2.1, 10.0.0.1")

		ip := GetClientIP(req, trustedProxies, "")
		assert.Equal(t, "198.51.100.99", ip, "Should extract leftmost (original client) IP from chain")
	})
}

// Benchmark tests to ensure performance
func BenchmarkGetClientIP_DirectConnection(b *testing.B) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetClientIP(req, nil, "")
	}
}

func BenchmarkGetClientIP_WithTrustedProxy(b *testing.B) {
	trustedProxies, _ := ParseCIDRs([]string{"10.0.0.1"})
	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Real-IP", "5.5.5.5")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetClientIP(req, trustedProxies, "X-Real-IP")
	}
}

func BenchmarkParseCIDRs(b *testing.B) {
	cidrs := []string{"192.168.1.0/24", "10.0.0.0/8", "172.16.0.0/12"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseCIDRs(cidrs)
	}
}

func BenchmarkIsIPInAnyCIDR(b *testing.B) {
	cidrs, _ := ParseCIDRs([]string{"192.168.1.0/24", "10.0.0.0/8", "172.16.0.0/12"})
	ip := net.ParseIP("10.5.10.20")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsIPInAnyCIDR(ip, cidrs)
	}
}
