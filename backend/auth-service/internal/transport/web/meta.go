package web

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

func userAgent(r *http.Request) *string {
	ua := r.Header.Get("User-Agent")
	if ua == "" {
		return nil
	}
	return &ua
}

func clientIP(r *http.Request) *netip.Addr {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first := strings.TrimSpace(strings.Split(xff, ",")[0])
		if addr, err := netip.ParseAddr(first); err == nil {
			return &addr
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return &addr
	}
	return nil

}
