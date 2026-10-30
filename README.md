# uwebsocket

A Gorilla WebSocket hub integrated with `github.com/dunv/uhttp`.
Development uses Go 1.27.3; consumers require Go 1.27 or newer.

## Delivery modes

Existing `WithWelcomeMessages` handlers retain best-effort delivery: live messages
can precede the welcome, and a full client queue drops messages, including welcome
messages. Welcome construction now runs asynchronously per client, outside the hub
event loop. An initialization error in this legacy mode is logged, not treated as
a disconnect. Builders must honor the client context.

`WithInitialSnapshot` is an opt-in alternative for authoritative initial state
followed by incremental updates. The hub keeps the client out of live delivery
until the application establishes a consistent snapshot boundary. It then retains
matching increments, queues the snapshot before them, and transitions to live
operation. The application must replace its subscribed scope on every initial
snapshot, including an explicitly empty one. Reconnect creates a new boundary;
this library does not define entity revisions, cross-replica ordering or replay.

### Establishing the boundary

The builder receives `begin`, which must be called exactly once while holding the
same application lock used for state mutation and synchronous `hub.Send` publication.
Capture a deep/immutable copy before releasing that lock, then do expensive encoding
outside it. Updates published before `begin` are covered by the snapshot; updates
after capture are retained as increments. Merely buffering while an arbitrary
welcome callback executes cannot provide this consistency.

For example, given a cache whose `Snapshot` returns an independent copy:

```go
handler := uwebsocket.NewHandler(
    uwebsocket.WithInitialSnapshot(
        uwebsocket.SnapshotLimits{
            Timeout:     15 * time.Second,
            MaxMessages: 256,
            MaxBytes:    4 << 20,
        },
        func(hub uwebsocket.WebSocketHub, id string,
            attrs *uwebsocket.ClientAttributes, r *http.Request,
            ctx context.Context, begin func() error,
        ) ([][]byte, error) {
            publicationMu.Lock()
            if err := begin(); err != nil {
                publicationMu.Unlock()
                return nil, err
            }
            snapshot := cache.Snapshot() // apply the client's authorized scope
            publicationMu.Unlock()

            if err := ctx.Err(); err != nil {
                return nil, err
            }
            payload, err := json.Marshal(snapshot)
            if err != nil {
                return nil, err
            }
            return [][]byte{payload}, nil
        },
    ),
)
```

All writers participating in that view use the same boundary:

```go
publicationMu.Lock()
cache.Apply(change)
hub.Send(
    uwebsocket.WithFilterFn(matchesScope),
    uwebsocket.WithMessage(encodedIncrement),
)
publicationMu.Unlock()
```

The application owns scope authorization, consistent state capture, immutable
payloads and publication ordering. Do not publish outside that boundary or mutate
queued byte slices. Do not send snapshot messages through `hub.Send` from the
builder: return them. `WithWelcomeMessages` and `WithInitialSnapshot` are mutually
exclusive; invalid options panic during handler construction.

## Bounds and failure

Zero snapshot limits default to 15 seconds, 256 queued messages and 4 MiB of queued
payload bytes per client. The initial snapshot and retained increments must fit
those limits together. The message/byte limits continue to apply to the live
outbound queue; one frame already taken by the writer is outside that queue.
Choose limits for representative scoped snapshots and burst sizes, not as a
latency/delivery guarantee.

Initialization error, missing boundary, timeout, encoding failure or queue overflow
removes/cancels the opted-in client rather than silently skipping required state.
The browser must reconnect and reinitialize. The builder must honor its context:
Go cannot forcibly terminate a callback which ignores cancellation, although the
hub removes its client on timeout without waiting for that callback.

Use the hub's `Send` API. Direct `WebSocketClient.SendChan` writes bypass ordering
and budget accounting and are unsupported for snapshot-mode clients. Filters run
under the registry lock and must be fast, read-only and non-reentrant. Configure
client attributes before registration; concurrent attribute mutation is unsupported.
Message builders execute synchronously and once per send with interested recipients,
serialized against other sends, but outside the client registry lock. Initial
snapshot builders run separately from the hub event loop, so slow initialization
does not stop registration, unregistration or normal delivery to established clients.

A queued message is not proof of network delivery or application acknowledgement.
The hub is not a durable event log, a distributed cache or a reconnect replay service.
Snapshot builders and their state locks must not block indefinitely.

## Origin policy

Browser handshakes are checked against the actual `Origin` header, not the server's
request Host. Configure origins through uhttp's CORS setting:

- Empty: require the same scheme, host and effective port as the request.
- Comma-separated full origins: an explicit allowlist, including scheme and port.
  Default ports are normalized. This list also restricts same-origin requests.
- Bare host/port entries remain supported and allow either HTTP or HTTPS. Prefer
  full origins when scheme restriction matters.
- `*`: explicitly allow any valid HTTP(S) origin and the opaque `null` origin.

Absent Origin is allowed for non-browser clients; empty, duplicate and malformed
Origin values are rejected. Origin policy is not authentication. Forwarded headers
are not trusted: TLS-terminating proxies should configure the public browser origins
explicitly rather than rely on the empty-policy transport comparison.

## Callback scheduling

Incoming-message callbacks run FIFO within each client, but different clients may
execute callbacks concurrently. Synchronize shared application state accordingly;
a stalled callback no longer blocks registration, outgoing delivery or other clients.
Each client has a bounded waiting queue (64 messages by default), configurable with
`WithIncomingMessageQueue`. The reader limits each message to 512 bytes; one active
callback is outside the waiting queue. Queue overflow disconnects that client rather
than dropping input or blocking the hub loop. Queued callbacks are skipped after
cancellation. Work already started is not undone; do not blindly replay mutations
following a disconnect.

Callbacks/builders must honor their context. Go cannot forcibly stop application
code which ignores cancellation. `WithOnConnect` continues to execute in the HTTP
upgrade handler, not the hub loop. Sending legacy welcome fragments and live messages
may interleave; only `WithInitialSnapshot` establishes ordered initialization.

## Verification

```sh
go mod verify
go vet ./...
go test -race -count=1 ./...
go test -run '^$' -bench BenchmarkHubSend -benchmem ./...
```

Tests cover scope filtering, shared payload construction, legacy overflow, explicit
snapshot boundaries, initial-before-live ordering, no duplicate append batches,
initialization failure/timeout, byte/count limits, cancellation, real-socket
reconnect, Origin rejection, callback ordering and overloaded-client isolation.
A suite-wide goroutine-leak check verifies cleanup. The send benchmark checks
shared/no-subscriber build behavior and cost; it is not an end-to-end browser or
multi-replica benchmark. CI runs race tests and vet; CodeQL is a separate workflow.

## Migration notes

- The Go minimum is now 1.27. Direct/transitive dependencies and GitHub Actions
  were updated; consumers need their own integration verification before adoption.
- Origin allowlists now constrain the browser Origin, fixing the previous
  request-Host comparison. Review proxy/public-origin configuration. `*` remains
  an explicit unrestricted policy, not a secure browser allowlist.
- Welcome callbacks are asynchronous and best-effort welcome overflow drops instead
  of blocking the hub. Use snapshot mode when missing initialization is unacceptable.
- Incoming callbacks are serialized per client, not globally. Audit shared mutable
  state and handle input-overflow disconnects without assuming failed execution.
- Normal/going-away closes and intentional cancellation no longer report spurious
  errors. Unexpected protocol closes now reach `WithOnError` instead of being
  silently suppressed by the previous inverted classification.
- Attribute error sentinels gained idiomatic names; deprecated aliases preserve
  the existing error identities. Public hub/client interfaces are unchanged.
