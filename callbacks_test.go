package uwebsocket

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func receiveCallback(t *testing.T, events <-chan string, want string) {
	t.Helper()
	select {
	case got := <-events:
		require.Equal(t, want, got)
	case <-time.After(2 * time.Second):
		t.Fatal("callback timed out")
	}
}

func TestSlowLegacyWelcomeDoesNotBlockHub(t *testing.T) {
	h := snapshotHub(t)
	started, finished := make(chan struct{}), make(chan struct{})
	slow := addSnapshotClient(t, h, "slow", NewHandler(WithWelcomeMessages(
		func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, ctx context.Context) ([][]byte, error) {
			defer close(finished)
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		})))
	awaitSignal(t, started)
	other := addSnapshotClient(t, h, "other", NewHandler())
	h.Send(WithClientFilter(other.guid), WithMessage([]byte("unaffected")))
	readSnapshotMessage(t, other, "unaffected")
	h.unregisterClient(slow)
	awaitSignal(t, slow.ctx.Done())
	awaitSignal(t, finished)
	require.Equal(t, 1, h.CountClientsWithFilter(func(string, *ClientAttributes) bool { return true }))
}

func TestIncomingCallbacksAreFIFOAndIsolatedPerClient(t *testing.T) {
	h := snapshotHub(t)
	started, release := make(chan struct{}), make(chan struct{})
	events, otherEvents := make(chan string, 3), make(chan string, 1)
	handle := func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, msg ClientMessage, ctx context.Context) {
		events <- string(msg.Message)
		if string(msg.Message) == "one" {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
	}
	slow := addSnapshotClient(t, h, "slow", NewHandler(WithOnIncomingMessage(handle), WithIncomingMessageQueue(2)))
	require.True(t, h.dispatchIncoming(slow, []byte("one")))
	awaitSignal(t, started)
	require.True(t, h.dispatchIncoming(slow, []byte("two")))
	require.True(t, h.dispatchIncoming(slow, []byte("three")))
	other := addSnapshotClient(t, h, "other", NewHandler(WithOnIncomingMessage(
		func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, msg ClientMessage, _ context.Context) {
			otherEvents <- string(msg.Message)
		})))
	require.True(t, h.dispatchIncoming(other, []byte("independent")))
	receiveCallback(t, otherEvents, "independent")
	h.Send(WithClientFilter(other.guid), WithMessage([]byte("outgoing")))
	readSnapshotMessage(t, other, "outgoing")
	receiveCallback(t, events, "one")
	close(release)
	receiveCallback(t, events, "two")
	receiveCallback(t, events, "three")
}

func TestIncomingOverflowDisconnectsOnlyOffenderAndSkipsQueuedWork(t *testing.T) {
	h := snapshotHub(t)
	started, finished := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	slow := addSnapshotClient(t, h, "offender", NewHandler(WithIncomingMessageQueue(2), WithOnIncomingMessage(
		func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, _ ClientMessage, ctx context.Context) {
			calls.Add(1)
			close(started)
			<-ctx.Done()
			close(finished)
		})))
	other := addSnapshotClient(t, h, "other", NewHandler())
	require.True(t, h.dispatchIncoming(slow, []byte("active")))
	awaitSignal(t, started)
	require.True(t, h.dispatchIncoming(slow, []byte("queued-one")))
	require.True(t, h.dispatchIncoming(slow, []byte("queued-two")))
	require.False(t, h.dispatchIncoming(slow, []byte("overflow")))
	awaitSignal(t, slow.ctx.Done())
	awaitSignal(t, finished)
	require.False(t, h.dispatchIncoming(slow, []byte("after-disconnect")))
	require.Equal(t, int32(1), calls.Load())
	h.Send(WithMessage([]byte("still-connected")))
	readSnapshotMessage(t, other, "still-connected")
	require.Equal(t, 1, h.CountClientsWithFilter(func(string, *ClientAttributes) bool { return true }))
}

func TestLegacyWelcomeErrorRemainsBestEffort(t *testing.T) {
	h := snapshotHub(t)
	c := addSnapshotClient(t, h, "legacy", NewHandler())
	h.clientLock.Lock()
	registered := h.clients[c.guid]
	h.clientLock.Unlock()
	h.sendWelcome(registered, func(WebSocketHub, string, *ClientAttributes, *http.Request, context.Context) ([][]byte, error) {
		return nil, errors.New("welcome failed")
	})
	h.Send(WithMessage([]byte("still-live")))
	readSnapshotMessage(t, c, "still-live")
}

func TestLegacyWelcomeOverflowDoesNotBlockRegistration(t *testing.T) {
	h := snapshotHub(t)
	c := addSnapshotClient(t, h, "legacy", NewHandler())
	h.clientLock.Lock()
	registered := h.clients[c.guid]
	h.clientLock.Unlock()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		h.sendWelcome(registered, func(WebSocketHub, string, *ClientAttributes, *http.Request, context.Context) ([][]byte, error) {
			messages := make([][]byte, cap(c.send)+1)
			for i := range messages {
				messages[i] = []byte("welcome")
			}
			return messages, nil
		})
	}()
	awaitSignal(t, finished)
	h.clientLock.Lock()
	discarded := h.discardedMessages
	h.clientLock.Unlock()
	require.Equal(t, int64(1), discarded)
	other := addSnapshotClient(t, h, "other", NewHandler())
	h.Send(WithClientFilter(other.guid), WithMessage([]byte("registered")))
	readSnapshotMessage(t, other, "registered")
}

func TestIncomingOverflowClosesActualSocketWithoutAffectingOtherClients(t *testing.T) {
	h := snapshotHub(t)
	started, finished := make(chan struct{}), make(chan struct{})
	h.Handle("/input", NewHandler(WithIncomingMessageQueue(1), WithOnIncomingMessage(
		func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, msg ClientMessage, ctx context.Context) {
			if string(msg.Message) != "active" {
				t.Error("canceled client's queued callback executed")
				return
			}
			close(started)
			<-ctx.Done()
			close(finished)
		})))
	server := httptest.NewServer(h.u.ServeMux())
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/input"
	offender, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	defer func() { _ = offender.Close() }()
	require.NoError(t, offender.WriteMessage(websocket.TextMessage, []byte("active")))
	awaitSignal(t, started)
	healthy, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	defer func() { _ = healthy.Close() }()
	ping := func() {
		require.NoError(t, healthy.SetReadDeadline(time.Now().Add(time.Second)))
		require.NoError(t, healthy.WriteMessage(websocket.TextMessage, []byte("PING")))
		_, message, err := healthy.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, "PONG", string(message))
	}
	ping()
	require.NoError(t, offender.WriteMessage(websocket.TextMessage, []byte("queued")))
	require.NoError(t, offender.WriteMessage(websocket.TextMessage, []byte("overflow")))
	require.NoError(t, offender.SetReadDeadline(time.Now().Add(time.Second)))
	_, _, err = offender.ReadMessage()
	var closeError *websocket.CloseError
	require.ErrorAs(t, err, &closeError, "overflow must close the connection, not merely stall it")
	awaitSignal(t, finished)
	ping()
}

func TestIncomingQueueValidation(t *testing.T) {
	require.Panics(t, func() { WithIncomingMessageQueue(0) })
	require.Panics(t, func() { WithIncomingMessageQueue(-1) })
}
