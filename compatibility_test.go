package uwebsocket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestLegacyWelcomeRetainsBestEffortOrdering(t *testing.T) {
	h := snapshotHub(t)
	building, release := make(chan struct{}), make(chan struct{})
	h.Handle("/legacy", NewHandler(WithWelcomeMessages(
		func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, ctx context.Context) ([][]byte, error) {
			close(building)
			select {
			case <-release:
				return [][]byte{[]byte("welcome")}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		})))
	server := httptest.NewServer(h.u.ServeMux())
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/legacy", nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	awaitSignal(t, building)
	h.Send(WithMessage([]byte("live-before-welcome")))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	_, message, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "live-before-welcome", string(message))
	close(release)
	_, message, err = conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "welcome", string(message))
}

func TestTextPingUsesBoundedSnapshotQueue(t *testing.T) {
	h := snapshotHub(t)
	h.Handle("/ping", NewHandler(WithInitialSnapshot(SnapshotLimits{MaxBytes: 4},
		func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, _ context.Context, begin func() error) ([][]byte, error) {
			return [][]byte{[]byte("[]")}, begin()
		})))
	server := httptest.NewServer(h.u.ServeMux())
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ping", nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	_, initial, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "[]", string(initial))
	for i := 0; i < 10; i++ {
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("PING")))
		_, message, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, "PONG", string(message))
	}
}

func TestDisconnectDuringMessageConstruction(t *testing.T) {
	h := snapshotHub(t)
	removed := addSnapshotClient(t, h, "removed", NewHandler())
	remaining := addSnapshotClient(t, h, "remaining", NewHandler())
	building, release, sent := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(sent)
		h.Send(WithMessageFn(func() ([]byte, error) {
			close(building)
			<-release
			return []byte("payload"), nil
		}))
	}()
	awaitSignal(t, building)
	h.unregisterClient(removed)
	awaitSignal(t, removed.ctx.Done())
	close(release)
	awaitSignal(t, sent)
	readSnapshotMessage(t, remaining, "payload")
	_, ok := <-removed.send
	require.False(t, ok, "must not send into a channel closed during construction")
}

func TestCloseErrorClassification(t *testing.T) {
	for _, code := range []int{websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseProtocolError} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			h := snapshotHub(t)
			connected := make(chan context.Context, 1)
			reported := make(chan error, 4)
			h.Handle("/close", NewHandler(
				WithOnConnect(func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, ctx context.Context) {
					connected <- ctx
				}),
				WithOnError(func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, err error, _ context.Context) {
					reported <- err
				}),
			))
			server := httptest.NewServer(h.u.ServeMux())
			defer server.Close()
			conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/close", nil)
			require.NoError(t, err)
			defer func() { _ = conn.Close() }()
			var ctx context.Context
			select {
			case ctx = <-connected:
			case <-time.After(time.Second):
				t.Fatal("connection callback timed out")
			}
			require.NoError(t, conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, "test"), time.Now().Add(time.Second)))
			awaitSignal(t, ctx.Done())
			select {
			case err := <-reported:
				require.Equal(t, websocket.CloseProtocolError, code, "normal closes must not report errors")
				var closeError *websocket.CloseError
				require.ErrorAs(t, err, &closeError)
				require.Equal(t, code, closeError.Code)
			default:
				require.NotEqual(t, websocket.CloseProtocolError, code, "unexpected protocol close must be reported")
			}
		})
	}
}

func TestAttributeErrorsRemainCompatible(t *testing.T) {
	attrs := NewClientAttributes()
	_, err := attrs.GetString("missing")
	require.ErrorIs(t, err, KEY_DOES_NOT_EXIST_ERR)
	attrs.SetBool("flag", true)
	_, err = attrs.GetString("flag")
	require.ErrorIs(t, err, KEY_HAS_WRONG_TYPE_ERR)
}
