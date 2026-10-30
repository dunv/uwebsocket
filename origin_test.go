package uwebsocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dunv/uhttp"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestOriginPolicy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		allowed string
		host    string
		secure  bool
		origins []string
		want    bool
	}{
		{name: "non-browser", allowed: "https://trusted.example", host: "server.example", want: true},
		{name: "default same origin", host: "APP.example", origins: []string{"http://app.example"}, want: true},
		{name: "default HTTPS", host: "app.example", secure: true, origins: []string{"https://app.example"}, want: true},
		{name: "default scheme mismatch", host: "app.example", secure: true, origins: []string{"http://app.example"}},
		{name: "TLS proxy requires explicit origin", allowed: "https://app.example", host: "app.example", origins: []string{"https://app.example"}, want: true},
		{name: "default equivalent HTTPS port", host: "app.example:443", secure: true, origins: []string{"https://app.example"}, want: true},
		{name: "default cross host", host: "server.example", origins: []string{"https://evil.example"}},
		{name: "explicit full origin", allowed: "https://trusted.example", host: "server.example", origins: []string{"https://trusted.example"}, want: true},
		{name: "list whitespace", allowed: " https://first.example , https://trusted.example/ ", host: "server.example", origins: []string{"https://trusted.example"}, want: true},
		{name: "wrong scheme", allowed: "https://trusted.example", host: "server.example", origins: []string{"http://trusted.example"}},
		{name: "default port", allowed: "https://trusted.example:443", host: "server.example", origins: []string{"https://trusted.example"}, want: true},
		{name: "explicit port", allowed: "https://trusted.example:8443", host: "server.example", origins: []string{"https://trusted.example:8443"}, want: true},
		{name: "wrong port", allowed: "https://trusted.example:8443", host: "server.example", origins: []string{"https://trusted.example"}},
		{name: "bare host compatibility", allowed: "trusted.example", host: "server.example", origins: []string{"https://trusted.example"}, want: true},
		{name: "request Host is not Origin", allowed: "trusted.example", host: "trusted.example", origins: []string{"https://evil.example"}},
		{name: "configured list restricts same host too", allowed: "https://trusted.example", host: "server.example", origins: []string{"https://server.example"}},
		{name: "host suffix attack", allowed: "https://trusted.example", host: "server.example", origins: []string{"https://trusted.example.evil.example"}},
		{name: "IPv6", allowed: "https://[::1]:8443", host: "server.example", origins: []string{"https://[::1]:8443"}, want: true},
		{name: "wildcard opt in", allowed: "*", host: "server.example", origins: []string{"https://other.example"}, want: true},
		{name: "null wildcard opt in", allowed: "*", host: "server.example", origins: []string{"null"}, want: true},
		{name: "null restricted", allowed: "https://trusted.example", host: "server.example", origins: []string{"null"}},
		{name: "empty Origin", allowed: "*", host: "server.example", origins: []string{""}},
		{name: "duplicate Origin", allowed: "*", host: "server.example", origins: []string{"https://trusted.example", "https://evil.example"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewWebSocketHub(uhttp.NewUHTTP(uhttp.WithCORS(tc.allowed)), websocket.TextMessage, t.Context()).(*webSocketHub)
			scheme := "http"
			if tc.secure {
				scheme = "https"
			}
			r := httptest.NewRequest(http.MethodGet, scheme+"://"+tc.host+"/", nil)
			for _, origin := range tc.origins {
				r.Header.Add("Origin", origin)
			}
			r.Header.Set("X-Forwarded-Host", "trusted.example")
			r.Header.Set("X-Forwarded-Proto", "https")
			require.Equal(t, tc.want, h.checkOrigin(r))
		})
	}
}

func TestMalformedOriginsRejectedEvenWithWildcard(t *testing.T) {
	h := NewWebSocketHub(uhttp.NewUHTTP(uhttp.WithCORS("*")), websocket.TextMessage, t.Context()).(*webSocketHub)
	for _, origin := range []string{
		"https://", "https://user@trusted.example", "https://trusted.example/path",
		"https://trusted.example?query", "https://trusted.example#fragment", "https://trusted.example#",
		"https://trusted.example:", "https://trusted.example:99999", "https://trusted.example:invalid",
		"file:///etc/passwd", "ftp://trusted.example", "https://first.example https://second.example",
		"https://first.example,https://second.example", "https://trusted.example\n",
	} {
		t.Run(origin, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://server.example/", nil)
			r.Header.Set("Origin", origin)
			require.False(t, h.checkOrigin(r))
		})
	}
}

func TestOriginPolicyAtUpgrade(t *testing.T) {
	h := snapshotHub(t)
	// No requests have started; replace the fixture's wildcard-configured server.
	h.u = uhttp.NewUHTTP(uhttp.WithCORS("https://trusted.example"))
	h.Handle("/origin", NewHandler())
	server := httptest.NewServer(h.u.ServeMux())
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/origin"
	for _, origin := range []string{"https://evil.example", "https://trusted.example"} {
		headers := http.Header{"Host": {"trusted.example"}, "Origin": {origin}}
		conn, response, err := websocket.DefaultDialer.Dial(url, headers)
		if origin == "https://evil.example" {
			require.Error(t, err)
			require.Nil(t, conn)
			require.NotNil(t, response)
			require.Equal(t, http.StatusForbidden, response.StatusCode)
			require.NoError(t, response.Body.Close())
		} else {
			require.NoError(t, err)
			require.NoError(t, conn.Close())
		}
	}
}
