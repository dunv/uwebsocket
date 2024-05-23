package uwebsocket

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/dunv/uhttp"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

var testClientKey = "testClientKey"
var testClientFlag = "testClientKeyFlag"
var testClientGUID1 = "testClientGUID1"
var testClientGUID2 = "testClientGUID2"
var testClient1Value = "testClientValue1"
var testClient2Value = "testClientValue2"
var message1 = []byte("message1")
var message2 = []byte("message2")
var message3 = []byte("message3")

func TestMessageFilter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	h, c1, c2 := createTestSetup(t, ctx)

	h.Send(WithMessage(message1), WithMatchFilter(testClientKey, testClient1Value))
	h.Send(WithMessage(message2), WithFlagFilter(testClientFlag))
	h.Send(WithMessage(message3), WithClientFilter(testClientGUID1))

	// client 1 received all msgs
	received, err := c1.readOne()
	require.NoError(t, err)
	require.Equal(t, message1, received)
	received, err = c1.readOne()
	require.NoError(t, err)
	require.Equal(t, message2, received)
	received, err = c1.readOne()
	require.NoError(t, err)
	require.Equal(t, message3, received)
	_, err = c1.readOne()
	require.Error(t, err)

	// client 2 received no msgs
	_, err = c2.readOne()
	require.Error(t, err)

	// verify that no messages were discarded
	require.Equal(t, int64(0), h.discardedMessages)
}

func TestMessageCache(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	h, c1, c2 := createTestSetup(t, ctx)

	calledMsg1Fn := 0
	msg1Fn := func() ([]byte, error) {
		calledMsg1Fn++
		return []byte(message1), nil
	}
	calledMsg2Fn := 0
	msg2Fn := func() ([]byte, error) {
		calledMsg2Fn++
		return []byte(message2), nil
	}

	h.Send(WithMessageFn(msg1Fn))
	h.Send(WithMessageFn(msg2Fn), WithFilterFn(func(clientGUID string, attrs *ClientAttributes) bool {
		return attrs.HasMatch(testClientKey, testClient2Value)
	}))

	// client 1 only received one msg
	received, err := c1.readOne()
	require.NoError(t, err)
	require.Equal(t, message1, received)
	_, err = c1.readOne()
	require.Error(t, err)

	// client 2 received both messages
	received, err = c2.readOne()
	require.NoError(t, err)
	require.Equal(t, message1, received)
	received, err = c2.readOne()
	require.NoError(t, err)
	require.Equal(t, message2, received)

	// all msgFns have only been called once (test msg-generation-cache)
	require.Equal(t, 1, calledMsg1Fn)
	require.Equal(t, 1, calledMsg2Fn)

	// verify that no messages were discarded
	require.Equal(t, int64(0), h.discardedMessages)
}

func TestMessageBuffer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	h, c1, _ := createTestSetup(t, ctx)

	h.Send(WithMessage(message1), WithClientFilter(testClientGUID1))
	h.Send(WithMessage(message1), WithClientFilter(testClientGUID1))
	h.Send(WithMessage(message1), WithClientFilter(testClientGUID1))
	h.Send(WithMessage(message1), WithClientFilter(testClientGUID1))
	h.Send(WithMessage(message1), WithClientFilter(testClientGUID1))

	// client 1 received all msgs
	received, err := c1.readOne()
	require.NoError(t, err)
	require.Equal(t, message1, received)
	received, err = c1.readOne()
	require.NoError(t, err)
	require.Equal(t, message1, received)
	received, err = c1.readOne()
	require.NoError(t, err)
	require.Equal(t, message1, received)

	// buffer was full, two messages were skipped
	_, err = c1.readOne()
	require.Error(t, err)

	// buffer is free again
	h.Send(WithMessage(message2), WithClientFilter(testClientGUID1))
	received, err = c1.readOne()
	require.NoError(t, err)
	require.Equal(t, message2, received)

	// verify that two messages were discarded
	require.Equal(t, int64(2), h.discardedMessages)
}

func createTestSetup(t *testing.T, ctx context.Context) (*webSocketHub, *WebSocketClientMock, *WebSocketClientMock) {
	hInt := NewWebSocketHub(uhttp.NewUHTTP(), websocket.TextMessage, ctx)
	h := reflect.ValueOf(hInt).Interface().(*webSocketHub)
	go h.Run()
	client1 := NewWebSocketClientMock(t, ctx, testClientGUID1,
		NewClientAttributes().SetString(testClientKey, testClient1Value).SetBool(testClientFlag, true),
	)
	client2 := NewWebSocketClientMock(t, ctx, testClientGUID2,
		NewClientAttributes().SetString(testClientKey, testClient2Value),
	)

	select {
	case <-ctx.Done():
		t.Error("timeout when registering client1")
		t.FailNow()
	case h.register <- client1:
	}
	select {
	case <-ctx.Done():
		t.Error("timeout when registering client2")
		t.FailNow()
	case h.register <- client2:
	}

	for {
		if ctx.Err() != nil {
			t.Error("timeout when waiting for clients to register")
			t.FailNow()
		}

		count := h.CountClientsWithFilter(func(clientGUID string, attrs *ClientAttributes) bool { return true })
		if count == 2 {
			t.Log("Clients registered")
			return h, client1, client2
		}
		time.Sleep(10 * time.Millisecond)
	}

}
