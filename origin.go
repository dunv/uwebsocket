package uwebsocket

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// CORS is an Origin allowlist, not an allowlist of the server's request Host.
// Empty configuration requires the request's scheme/host/port; non-browser
// clients without Origin remain allowed. Forwarded headers are not trusted.
func (h *webSocketHub) checkOrigin(r *http.Request) bool {
	values := r.Header.Values("Origin")
	if len(values) == 0 {
		return true
	}
	if len(values) != 1 {
		return false
	}
	origin := values[0]
	allowed := strings.TrimSpace(h.u.CORS())
	if origin == "null" {
		return allowed == "*"
	}
	parsed, ok := parseOrigin(origin)
	if !ok {
		return false
	}
	if allowed == "*" {
		return true
	}
	if allowed == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		requestOrigin, valid := parseOrigin(scheme + "://" + r.Host)
		return valid && sameOrigin(parsed, requestOrigin)
	}
	for _, entry := range strings.Split(allowed, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		// Bare hosts retain host-only configuration compatibility; prefer full
		// origins when the scheme must also be restricted.
		if !strings.Contains(entry, "://") {
			entry = parsed.Scheme + "://" + entry
		}
		candidate, valid := parseOrigin(strings.TrimSuffix(entry, "/"))
		if valid && sameOrigin(parsed, candidate) {
			return true
		}
	}
	return false
}

func parseOrigin(raw string) (*url.URL, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || strings.ContainsAny(raw, "?#") || u.Opaque != "" || strings.HasSuffix(u.Host, ":") {
		return nil, false
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, false
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 0 || port > 65535 {
			return nil, false
		}
	}
	return u, true
}

func sameOrigin(a, b *url.URL) bool {
	address := func(u *url.URL) string {
		port := u.Port()
		if port == "" {
			port = "80"
			if u.Scheme == "https" {
				port = "443"
			}
		}
		return net.JoinHostPort(strings.ToLower(u.Hostname()), port)
	}
	return a.Scheme == b.Scheme && address(a) == address(b)
}
