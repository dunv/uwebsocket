package uwebsocket

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dunv/uhttp"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type snapshotTestClient struct {
	hub     *webSocketHub
	guid    string
	attrs   *ClientAttributes
	send    chan []byte
	ctx     context.Context
	cancel  context.CancelFunc
	handler Handler
	running chan struct{}
}

func (c *snapshotTestClient) ClientGUID() string            { return c.guid }
func (c *snapshotTestClient) Attributes() *ClientAttributes { return c.attrs }
func (c *snapshotTestClient) SendChan() chan []byte         { return c.send }
func (c *snapshotTestClient) Ctx() context.Context          { return c.ctx }
func (c *snapshotTestClient) Cancel()                       { c.cancel() }
func (c *snapshotTestClient) Handler() Handler              { return c.handler }
func (c *snapshotTestClient) Request() *http.Request        { return nil }
func (c *snapshotTestClient) Run(context.Context)           { close(c.running) }

func snapshotHub(t *testing.T) *webSocketHub {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	h := NewWebSocketHub(uhttp.NewUHTTP(uhttp.WithCORS("*")), websocket.TextMessage, ctx).(*webSocketHub)
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.Run()
	}()
	t.Cleanup(func() {
		cancel()
		awaitSignal(t, done)
	})
	return h
}

func addSnapshotClient(t *testing.T, h *webSocketHub, guid string, handler Handler) *snapshotTestClient {
	t.Helper()
	ctx, cancel := context.WithCancel(h.ctx)
	c := &snapshotTestClient{
		hub: h, guid: guid, attrs: NewClientAttributes(), send: make(chan []byte, 256),
		ctx: ctx, cancel: cancel, handler: handler, running: make(chan struct{}),
	}
	select {
	case h.register <- c:
	case <-time.After(2 * time.Second):
		t.Fatal("registration blocked")
	}
	awaitSignal(t, c.running)
	return c
}

func awaitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("signal timed out")
	}
}

func readSnapshotMessage(t *testing.T, c *snapshotTestClient, want string) {
	t.Helper()
	select {
	case message, ok := <-c.send:
		require.True(t, ok, "client unexpectedly disconnected")
		c.hub.messageDequeued(c, len(message))
		require.Equal(t, want, string(message))
	case <-time.After(2 * time.Second):
		t.Fatal("message timed out")
	}
}

func TestSnapshotBoundaryAndIncrementalHandoff(t *testing.T) {
	h := snapshotHub(t)
	var mu sync.Mutex
	state := "before"
	entered, capture, captured, encode := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	builder := func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, ctx context.Context, begin func() error) ([][]byte, error) {
		close(entered)
		select {
		case <-capture:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		mu.Lock()
		err := begin()
		snapshot := state
		mu.Unlock()
		close(captured)
		if err != nil {
			return nil, err
		}
		select {
		case <-encode:
			return [][]byte{[]byte(snapshot)}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	c := addSnapshotClient(t, h, "initializing", NewHandler(WithInitialSnapshot(SnapshotLimits{}, builder)))
	awaitSignal(t, entered)
	// A pre-boundary update is included in the snapshot, not replayed as a delta.
	mu.Lock()
	state = "snapshot"
	called := false
	h.Send(WithMessageFn(func() ([]byte, error) { called = true; return []byte(state), nil }))
	mu.Unlock()
	require.False(t, called, "no interested recipient before its boundary")
	close(capture)
	awaitSignal(t, captured)
	mu.Lock()
	state = "delta-one"
	h.Send(WithMessage([]byte(state)))
	state = "delta-two"
	h.Send(WithMessage([]byte(state)))
	mu.Unlock()
	h.Send(WithClientFilter("different-scope"), WithMessage([]byte("not-for-client")))
	select {
	case <-c.send:
		t.Fatal("live message overtook initial snapshot")
	default:
	}
	close(encode)
	readSnapshotMessage(t, c, "snapshot")
	readSnapshotMessage(t, c, "delta-one")
	readSnapshotMessage(t, c, "delta-two")
	h.Send(WithMessage([]byte("live")))
	readSnapshotMessage(t, c, "live")
	select {
	case <-c.send:
		t.Fatal("unexpected duplicate or wrong-scope message")
	default:
	}
}

func TestSnapshotDoesNotBlockOtherClientsOrRegistry(t *testing.T) {
	h := snapshotHub(t)
	captured, release := make(chan struct{}), make(chan struct{})
	builder := func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, ctx context.Context, begin func() error) ([][]byte, error) {
		if err := begin(); err != nil {
			return nil, err
		}
		close(captured)
		select {
		case <-release:
			return [][]byte{[]byte("snapshot")}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	initializing := addSnapshotClient(t, h, "slow", NewHandler(WithInitialSnapshot(SnapshotLimits{}, builder)))
	awaitSignal(t, captured)
	established := addSnapshotClient(t, h, "established", NewHandler())
	h.Send(WithMessage([]byte("delta")))
	readSnapshotMessage(t, established, "delta")
	// A slow message builder also cannot hold the registry lock.
	building, finish, sent := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(sent)
		h.Send(WithMessageFn(func() ([]byte, error) {
			close(building)
			<-finish
			return []byte("built"), nil
		}))
	}()
	awaitSignal(t, building)
	require.Equal(t, 2, h.CountClientsWithFilter(func(string, *ClientAttributes) bool { return true }))
	close(finish)
	awaitSignal(t, sent)
	readSnapshotMessage(t, established, "built")
	close(release)
	readSnapshotMessage(t, initializing, "snapshot")
	readSnapshotMessage(t, initializing, "delta")
	readSnapshotMessage(t, initializing, "built")
}

func TestSnapshotInitializationFailures(t *testing.T) {
	for _, kind := range []string{"builder-error", "no-boundary", "duplicate-boundary", "timeout", "disconnect", "pending-count", "pending-bytes", "snapshot-count", "snapshot-bytes", "encoding-error"} {
		t.Run(kind, func(t *testing.T) {
			h := snapshotHub(t)
			captured, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			limits := SnapshotLimits{Timeout: time.Second, MaxMessages: 3, MaxBytes: 12}
			if kind == "timeout" {
				limits.Timeout = 25 * time.Millisecond
			}
			builder := func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, ctx context.Context, begin func() error) ([][]byte, error) {
				defer close(finished)
				if kind == "no-boundary" {
					return nil, nil
				}
				if err := begin(); err != nil {
					return nil, err
				}
				close(captured)
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				switch kind {
				case "builder-error":
					return nil, errors.New("failed snapshot read")
				case "duplicate-boundary":
					return nil, begin()
				case "snapshot-count":
					return [][]byte{[]byte("a"), []byte("b"), []byte("c"), []byte("d")}, nil
				case "snapshot-bytes":
					return [][]byte{[]byte("snapshot")}, nil // retained delta pushes combined bytes over limit
				default:
					return [][]byte{[]byte("initial")}, nil
				}
			}
			c := addSnapshotClient(t, h, "failing", NewHandler(WithInitialSnapshot(limits, builder)))
			if kind != "no-boundary" {
				awaitSignal(t, captured)
			}
			switch kind {
			case "pending-count":
				for i := 0; i < 4; i++ {
					h.Send(WithMessage([]byte("x")))
				}
			case "pending-bytes":
				h.Send(WithMessage([]byte("too many bytes")))
			case "snapshot-bytes":
				h.Send(WithMessage([]byte("delta")))
			case "encoding-error":
				h.Send(WithMessageFn(func() ([]byte, error) { return nil, errors.New("failed delta encoding") }))
			case "disconnect":
				h.unregisterClient(c)
				awaitSignal(t, c.ctx.Done())
			}
			if kind != "timeout" {
				close(release)
			}
			awaitSignal(t, c.ctx.Done())
			awaitSignal(t, finished)
			_, ok := <-c.send
			require.False(t, ok, "failed initialization must not emit a partial snapshot")
			require.Zero(t, h.CountClientsWithFilter(func(string, *ClientAttributes) bool { return true }))
		})
	}
}

func TestSnapshotLiveOverflowDisconnectsOnlyOptedInClient(t *testing.T) {
	h := snapshotHub(t)
	builder := func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, _ context.Context, begin func() error) ([][]byte, error) {
		return [][]byte{[]byte("[]")}, begin()
	}
	c := addSnapshotClient(t, h, "bounded", NewHandler(WithInitialSnapshot(SnapshotLimits{}, builder)))
	readSnapshotMessage(t, c, "[]")
	for i := 0; i <= cap(c.send); i++ {
		h.Send(WithClientFilter(c.guid), WithMessage([]byte("delta")))
	}
	awaitSignal(t, c.ctx.Done())
	require.Zero(t, h.CountClientsWithFilter(func(string, *ClientAttributes) bool { return true }))
	h.clientLock.Lock()
	require.Zero(t, h.discardedMessages, "required live data must not be silently discarded")
	h.clientLock.Unlock()
}

func TestSnapshotOptionValidation(t *testing.T) {
	builder := func(WebSocketHub, string, *ClientAttributes, *http.Request, context.Context, func() error) ([][]byte, error) {
		return nil, nil
	}
	require.Panics(t, func() { WithInitialSnapshot(SnapshotLimits{}, nil) })
	for _, limits := range []SnapshotLimits{{Timeout: -1}, {MaxMessages: -1}, {MaxBytes: -1}} {
		require.Panics(t, func() { WithInitialSnapshot(limits, builder) })
	}
	welcome := WithWelcomeMessages(func(WebSocketHub, string, *ClientAttributes, *http.Request, context.Context) ([][]byte, error) {
		return nil, nil
	})
	require.Panics(t, func() { NewHandler(welcome, WithInitialSnapshot(SnapshotLimits{}, builder)) })
	require.Panics(t, func() { NewHandler(WithInitialSnapshot(SnapshotLimits{}, builder), welcome) })
}

func TestSnapshotTimeoutClosesActualSocketAndAllowsReconnect(t *testing.T) {
	h := snapshotHub(t)
	var mu sync.Mutex
	attempt := 0
	h.Handle("/snapshot", NewHandler(WithInitialSnapshot(SnapshotLimits{Timeout: 50 * time.Millisecond},
		func(_ WebSocketHub, _ string, _ *ClientAttributes, _ *http.Request, ctx context.Context, begin func() error) ([][]byte, error) {
			if err := begin(); err != nil {
				return nil, err
			}
			mu.Lock()
			attempt++
			current := attempt
			mu.Unlock()
			if current == 1 {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return [][]byte{[]byte("[]")}, nil
		})))
	server := httptest.NewServer(h.u.ServeMux())
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/snapshot"
	first, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	defer func() { _ = first.Close() }()
	require.NoError(t, first.SetReadDeadline(time.Now().Add(time.Second)))
	_, _, err = first.ReadMessage()
	require.Error(t, err)
	var closeError *websocket.CloseError
	require.ErrorAs(t, err, &closeError, "server must close, not merely leave the client to time out")
	second, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	defer func() { _ = second.Close() }()
	require.NoError(t, second.SetReadDeadline(time.Now().Add(time.Second)))
	_, message, err := second.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "[]", string(message))
	h.Send(WithMessage([]byte("increment")))
	_, message, err = second.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "increment", string(message))
}
