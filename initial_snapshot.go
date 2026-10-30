package uwebsocket

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// SnapshotLimits bound initialization time and each client's queued message
// count/bytes, including its snapshot and buffered increments. Zero fields use
// 15s, 256 messages and 4 MiB. Negative fields are invalid.
type SnapshotLimits struct {
	Timeout     time.Duration
	MaxMessages int
	MaxBytes    int
}

// SnapshotBuilder calls begin once while holding the application's state and
// publication lock, then captures an immutable snapshot before releasing that
// lock. Mutations and their synchronous hub.Send calls must use the same lock.
// Expensive encoding can happen afterward. The builder must honor ctx and must
// return messages rather than sending its snapshot through the hub.
type SnapshotBuilder func(hub WebSocketHub, clientGUID string, attrs *ClientAttributes, r *http.Request, ctx context.Context, begin func() error) ([][]byte, error)

type initialSnapshotOptions struct {
	limits SnapshotLimits
	build  SnapshotBuilder
}

// WithInitialSnapshot enables ordered snapshot-then-live delivery. It cannot be
// combined with WithWelcomeMessages. Initialization runs outside the hub event
// loop. Failure, timeout, or queue overflow disconnects the client; live overflow
// also disconnects instead of silently losing continuity. Reconnect must rebuild
// the client's authoritative scope. Existing handlers remain best-effort.
func WithInitialSnapshot(limits SnapshotLimits, build SnapshotBuilder) HandlerOption {
	if build == nil || limits.Timeout < 0 || limits.MaxMessages < 0 || limits.MaxBytes < 0 {
		panic("uwebsocket: invalid initial snapshot options")
	}
	if limits.Timeout == 0 {
		limits.Timeout = 15 * time.Second
	}
	if limits.MaxMessages == 0 {
		limits.MaxMessages = 256
	}
	if limits.MaxBytes == 0 {
		limits.MaxBytes = 4 << 20
	}
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.initialSnapshot = &initialSnapshotOptions{limits: limits, build: build}
	})
}

type snapshotPhase uint8

const (
	snapshotWaiting snapshotPhase = iota
	snapshotBuffering
	snapshotLive
)

// Protected by the hub's clientLock, including all outbound channel operations.
type snapshotDelivery struct {
	limits  SnapshotLimits
	phase   snapshotPhase
	pending [][]byte
	bytes   int
}

func (h *webSocketHub) startSnapshot(client *hubClient, opts *initialSnapshotOptions) {
	ctx, cancel := context.WithTimeout(client.ctx, opts.limits.Timeout)
	stop := context.AfterFunc(ctx, func() {
		h.clientLock.Lock()
		if client.active && client.snapshot.phase != snapshotLive {
			h.failDeliveryLocked(client, ctx.Err().Error())
		}
		h.clientLock.Unlock()
	})
	defer func() {
		stop()
		cancel()
	}()

	begin := func() error {
		h.clientLock.Lock()
		defer h.clientLock.Unlock()
		if err := ctx.Err(); err != nil {
			return err
		}
		if !client.active {
			return ErrClientNotFound
		}
		if client.snapshot.phase != snapshotWaiting {
			return errors.New("uwebsocket: snapshot boundary already established")
		}
		client.snapshot.phase = snapshotBuffering
		return nil
	}
	messages, err := opts.build(h, client.guid, client.attrs, client.client.Request(), ctx, begin)

	h.clientLock.Lock()
	defer h.clientLock.Unlock()
	if !client.active {
		return
	}
	delivery := client.snapshot
	if err == nil {
		err = ctx.Err()
	}
	if err == nil && delivery.phase != snapshotBuffering {
		err = errors.New("uwebsocket: snapshot builder did not establish a boundary")
	}
	size := delivery.bytes
	if err == nil {
		count := len(delivery.pending)
		for _, message := range messages {
			count++
			if len(message) > opts.limits.MaxBytes-size {
				err = errors.New("uwebsocket: initial snapshot byte limit exceeded")
				break
			}
			size += len(message)
		}
		if count > opts.limits.MaxMessages || count > cap(client.send)-len(client.send) {
			err = errors.New("uwebsocket: initial snapshot message limit exceeded")
		}
	}
	if err != nil {
		h.removeClientLocked(client)
		h.u.Log().Errorf("uwebsocket: initial snapshot client:%s err:%v", client.guid, err)
		return
	}
	// No live enqueue can interleave with this batch. Capacity was checked above;
	// the writer can only free space while clientLock is held.
	for _, message := range messages {
		client.send <- message
	}
	for _, message := range delivery.pending {
		client.send <- message
	}
	delivery.pending = nil
	delivery.bytes = size
	delivery.phase = snapshotLive
}

func validateSnapshotHandler(opts handlerOptions) {
	if opts.initialSnapshot != nil && opts.welcomeMessages != nil {
		panic("uwebsocket: WithInitialSnapshot and WithWelcomeMessages are mutually exclusive")
	}
}
