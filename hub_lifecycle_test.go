package uwebsocket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestSnapshotLiveByteBudgetIsReleasedByWriter(t *testing.T) {
	h := snapshotHub(t)
	builder := func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, _ context.Context, begin func() error) ([][]byte, error) {
		return [][]byte{[]byte("[]")}, begin()
	}
	c := addSnapshotClient(t, h, "budget", NewHandler(WithInitialSnapshot(SnapshotLimits{MaxBytes: 10}, builder)))
	other := addSnapshotClient(t, h, "other", NewHandler())
	readSnapshotMessage(t, c, "[]")
	for i := 0; i < 10; i++ {
		h.Send(WithClientFilter(c.guid), WithMessage([]byte("1234567890")))
		readSnapshotMessage(t, c, "1234567890")
	}
	h.Send(WithClientFilter(c.guid), WithMessage([]byte("12345")))
	h.Send(WithClientFilter(c.guid), WithMessage([]byte("12345")))
	h.Send(WithClientFilter(c.guid), WithMessage([]byte("x")))
	awaitSignal(t, c.ctx.Done())
	h.Send(WithMessage([]byte("unaffected")))
	readSnapshotMessage(t, other, "unaffected")
	require.Equal(t, 1, h.CountClientsWithFilter(func(string, *ClientAttributes) bool { return true }))
}

func TestSnapshotTimeoutDoesNotWaitForUncooperativeBuilder(t *testing.T) {
	h := snapshotHub(t)
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer func() {
		close(release)
		awaitSignal(t, finished)
	}()
	builder := func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, _ context.Context, begin func() error) ([][]byte, error) {
		defer close(finished)
		if err := begin(); err != nil {
			return nil, err
		}
		close(started)
		<-release
		return [][]byte{[]byte("must-not-be-published")}, nil
	}
	c := addSnapshotClient(t, h, "uncooperative", NewHandler(WithInitialSnapshot(SnapshotLimits{Timeout: 25 * time.Millisecond}, builder)))
	awaitSignal(t, started)
	awaitSignal(t, c.ctx.Done())
	_, ok := <-c.send
	require.False(t, ok)
	select {
	case <-finished:
		t.Fatal("test must exercise removal before builder returns")
	default:
	}
	other := addSnapshotClient(t, h, "healthy", NewHandler())
	h.Send(WithMessage([]byte("still-live")))
	readSnapshotMessage(t, other, "still-live")
}

func TestSendBuildersRemainSerializedOutsideRegistryLock(t *testing.T) {
	h := snapshotHub(t)
	c := addSnapshotClient(t, h, "recipient", NewHandler())
	entered, release, firstDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(firstDone)
		h.Send(WithMessageFn(func() ([]byte, error) {
			close(entered)
			<-release
			return []byte("first"), nil
		}))
	}()
	awaitSignal(t, entered)
	// This query used to deadlock inside a message builder's registry lock.
	require.Equal(t, 1, h.CountClientsWithFilter(func(string, *ClientAttributes) bool { return true }))
	secondStarted, secondDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(secondDone)
		close(secondStarted)
		h.Send(WithMessage([]byte("second")))
	}()
	awaitSignal(t, secondStarted)
	close(release)
	awaitSignal(t, firstDone)
	awaitSignal(t, secondDone)
	readSnapshotMessage(t, c, "first")
	readSnapshotMessage(t, c, "second")
}

func TestIncomingHandlerInstalledBeforeReadsAndRemovedOnDisconnect(t *testing.T) {
	h := snapshotHub(t)
	contexts := make(chan context.Context, 1)
	h.Handle("/incoming", NewHandler(WithOnIncomingMessage(func(hub WebSocketHub, id string, _ *ClientAttributes, _ *http.Request, msg ClientMessage, ctx context.Context) {
		contexts <- ctx
		hub.Send(WithClientFilter(id), WithMessage(msg.Message))
	})))
	server := httptest.NewServer(h.u.ServeMux())
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/incoming"
	for i := 0; i < 20; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		require.NoError(t, err)
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("early")))
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
		_, message, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, "early", string(message))
		var clientCtx context.Context
		select {
		case clientCtx = <-contexts:
		case <-time.After(time.Second):
			t.Fatal("incoming callback missing")
		}
		require.NoError(t, conn.Close())
		awaitSignal(t, clientCtx.Done())
		require.Zero(t, h.CountClientsWithFilter(func(string, *ClientAttributes) bool { return true }))
	}
}

func TestConcurrentSocketUpgrades(t *testing.T) {
	h := snapshotHub(t)
	h.Handle("/concurrent", NewHandler(WithInitialSnapshot(SnapshotLimits{},
		func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, _ context.Context, begin func() error) ([][]byte, error) {
			return [][]byte{[]byte("[]")}, begin()
		})))
	server := httptest.NewServer(h.u.ServeMux())
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/concurrent"
	var workers sync.WaitGroup
	for i := 0; i < 20; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			if !assertSocketResult(t, err) {
				return
			}
			defer func() { _ = conn.Close() }()
			if !assertSocketResult(t, conn.SetReadDeadline(time.Now().Add(time.Second))) {
				return
			}
			_, message, err := conn.ReadMessage()
			if assertSocketResult(t, err) && string(message) != "[]" {
				t.Errorf("unexpected initial message: %q", message)
			}
		}()
	}
	workers.Wait()
}

func assertSocketResult(t *testing.T, err error) bool {
	t.Helper()
	if err != nil {
		t.Error(err)
		return false
	}
	return true
}
