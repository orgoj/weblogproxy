package iputil

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// ParseCIDRs parses a list of string representations of IP addresses or CIDR notations.
func ParseCIDRs(cidrStrings []string) ([]*net.IPNet, error) {
	if len(cidrStrings) == 0 {
		return nil, nil
	}

	cidrs := make([]*net.IPNet, 0, len(cidrStrings))
	for _, cidrStr := range cidrStrings {
		// Check if it's a single IP address first
		ip := net.ParseIP(cidrStr)
		if ip != nil {
			// Convert single IP to CIDR mask (/32 for IPv4, /128 for IPv6)
			var mask net.IPMask
			if ip.To4() != nil {
				mask = net.CIDRMask(32, 32)
			} else {
				mask = net.CIDRMask(128, 128)
			}
			cidrs = append(cidrs, &net.IPNet{IP: ip, Mask: mask})
		} else {
			// Try parsing as CIDR
			_, ipNet, err := net.ParseCIDR(cidrStr)
			if err != nil {
				return nil, fmt.Errorf("invalid IP/CIDR format: %s (%w)", cidrStr, err)
			}
			cidrs = append(cidrs, ipNet)
		}
	}
	return cidrs, nil
}

// IsIPInAnyCIDR checks if the given IP address falls within any of the provided CIDR ranges.
func IsIPInAnyCIDR(ip net.IP, cidrs []*net.IPNet) bool {
	if ip == nil || len(cidrs) == 0 {
		return false
	}

	for _, cidr := range cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// GetClientIP extracts the client IP address from the request.
// It first tries the configured header (e.g. CF-Connecting-IP, X-Real-IP),
// then X-Forwarded-For (if remote IP is trusted), and finally falls back to RemoteAddr.
func GetClientIP(r *http.Request, trustedProxies []*net.IPNet, clientIPHeader string) string {
	// 1. Try configured header (e.g. CF-Connecting-IP, X-Real-IP, X-Client-Real-IP)
	// SECURITY: Only trust custom header if request came from a trusted proxy
	if clientIPHeader != "" {
		remoteIPStr, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			remoteIPStr = r.RemoteAddr
		}
		remoteIP := net.ParseIP(remoteIPStr)

		// Only trust the custom header if the immediate sender is in trusted_proxies
		if remoteIP != nil && IsIPInAnyCIDR(remoteIP, trustedProxies) {
			h := r.Header.Get(clientIPHeader)
			if h != "" {
				ip := strings.TrimSpace(h)
				if net.ParseIP(ip) != nil {
					return ip
				}
			}
		}
	}

	// 2. Try X-Forwarded-For if remote IP is trusted
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		remoteIPStr, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			remoteIPStr = r.RemoteAddr
		}
		remoteIP := net.ParseIP(remoteIPStr)
		if remoteIP != nil && IsIPInAnyCIDR(remoteIP, trustedProxies) {
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				firstIPStr := strings.TrimSpace(ips[0])
				if net.ParseIP(firstIPStr) != nil {
					return firstIPStr
				}
			}
		}
	}

	// 3. Fallback to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
