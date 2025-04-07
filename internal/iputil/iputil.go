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

// GetClientIP extracts the client IP address from the request, considering X-Forwarded-For.
// It needs the list of trusted proxy CIDRs to determine if X-Forwarded-For should be trusted.
func GetClientIP(r *http.Request, trustedProxies []*net.IPNet) string {
	// Try X-Forwarded-For first
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// Get the remote address (the immediate peer)
		remoteIPStr, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			// Log error? Fallback to RemoteAddr if split fails
			remoteIPStr = r.RemoteAddr
		}
		remoteIP := net.ParseIP(remoteIPStr)

		// Check if the remote peer is a trusted proxy
		if remoteIP != nil && IsIPInAnyCIDR(remoteIP, trustedProxies) {
			// Trust XFF header, take the *first* IP in the list
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				firstIPStr := strings.TrimSpace(ips[0])
				if net.ParseIP(firstIPStr) != nil {
					return firstIPStr // Return the first valid IP from XFF
				}
			}
		} // Else: Remote peer is not trusted, ignore XFF
	}

	// If XFF is not present or not trusted, use RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// This might happen for non-standard formats, return RemoteAddr as is
		return r.RemoteAddr
	}
	return ip
}
