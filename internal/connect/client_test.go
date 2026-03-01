package connect

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testToken = "a-valid-token"

// connectedClient brings up a client against ha and registers its shutdown.
func connectedClient(t *testing.T, ha *fakeHA, opts Options) *Client {
	t.Helper()

	c := newClientWithDialer(ha.dial, testToken, opts)
	require.NoError(t, c.Connect(context.Background()))
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestClientConnectAuthenticates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		connectedClient(t, ha, Options{})

		synctest.Wait()
		assert.Equal(t, 1, ha.dialCount())
	})
}

func TestClientConnectRejectsBadToken(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		c := newClientWithDialer(ha.dial, "wrong-token", Options{})

		err := c.Connect(context.Background())
		assert.ErrorIs(t, err, ErrAuthFailed)

		// A refused token must not leave the supervisor retrying in the
		// background, so there is nothing left to wait on.
		assert.NoError(t, c.Close())
	})
}

func TestClientConnectPropagatesDialFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		boom := errors.New("connection refused")
		ha.failDialsFrom(1, boom)

		c := newClientWithDialer(ha.dial, testToken, Options{})
		err := c.Connect(context.Background())

		assert.ErrorIs(t, err, boom)
		assert.NoError(t, c.Close())
	})
}

func TestClientSubscribeDeliversEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		c := connectedClient(t, ha, Options{})

		var got atomic.Int64
		require.NoError(t, c.Subscribe(Subscription{EventType: "state_changed"}, func(Message) {
			got.Add(1)
		}))

		synctest.Wait()
		conn := ha.current()
		subs := conn.subscriptions()
		require.Len(t, subs, 1)

		conn.emit(subs[0], "state_changed")
		synctest.Wait()

		assert.Equal(t, int64(1), got.Load())
	})
}

func TestClientIgnoresEventsForUnknownSubscriptions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		c := connectedClient(t, ha, Options{})

		var got atomic.Int64
		require.NoError(t, c.Subscribe(Subscription{EventType: "state_changed"}, func(Message) {
			got.Add(1)
		}))
		synctest.Wait()

		// An id nobody subscribed with must not reach any handler.
		ha.current().emit(9999, "state_changed")
		synctest.Wait()

		assert.Zero(t, got.Load())
	})
}

func TestClientCallCorrelatesResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		c := connectedClient(t, ha, Options{})

		msg, err := c.Call(context.Background(), mapRequest{"type": typePing})
		require.NoError(t, err)
		assert.Equal(t, typePong, msg.Type)
	})
}

func TestClientCallsGetDistinctIds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		c := connectedClient(t, ha, Options{})

		first, err := c.Call(context.Background(), mapRequest{"type": typePing})
		require.NoError(t, err)
		second, err := c.Call(context.Background(), mapRequest{"type": typePing})
		require.NoError(t, err)

		assert.NotEqual(t, first.ID, second.ID)
	})
}

func TestClientCallHonoursContextCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		c := connectedClient(t, ha, Options{})
		synctest.Wait()
		ha.current().ignorePingsFrom()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		_, err := c.Call(ctx, mapRequest{"type": typePing})
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func TestClientPendingCallsFailOnDisconnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		c := connectedClient(t, ha, Options{})
		synctest.Wait()

		conn := ha.current()
		conn.ignorePingsFrom()

		var (
			wg      sync.WaitGroup
			callErr error
		)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, callErr = c.Call(context.Background(), mapRequest{"type": typePing})
		}()

		// Let the call reach the wire before the connection dies under it.
		synctest.Wait()
		conn.serverClose()
		wg.Wait()

		// The id was only meaningful on the connection that just died, so the
		// caller has to be released rather than left waiting forever.
		assert.Error(t, callErr)
	})
}

func TestClientCallDeliversEachAnswerToItsOwnCaller(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const callers = 25

		ha := newFakeHA(t, testToken)
		c := connectedClient(t, ha, Options{PingInterval: time.Hour})
		synctest.Wait()

		var (
			wg         sync.WaitGroup
			mu         sync.Mutex
			mismatched []string
		)
		for i := range callers {
			wg.Add(1)
			go func() {
				defer wg.Done()

				marker := strconv.Itoa(i)
				msg, err := c.Call(context.Background(), mapRequest{
					"type":   "call_service",
					"marker": marker,
				})
				if err != nil {
					mu.Lock()
					mismatched = append(mismatched, "call "+marker+" failed: "+err.Error())
					mu.Unlock()
					return
				}

				var body struct {
					Result struct {
						Marker string `json:"marker"`
					} `json:"result"`
				}
				require.NoError(t, json.Unmarshal(msg.Raw, &body))

				if body.Result.Marker != marker {
					mu.Lock()
					mismatched = append(mismatched, "call "+marker+" got "+body.Result.Marker)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		// Asserting on the id alone would not catch this: a correlation that
		// handed each waiter an arbitrary pending answer still returns a
		// well-formed result to everybody.
		assert.Empty(t, mismatched, "every caller must receive the answer to its own request")
	})
}

func TestClientSendsIdsInIncreasingOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const senders = 50

		ha := newFakeHA(t, testToken)
		c := connectedClient(t, ha, Options{PingInterval: time.Hour})
		synctest.Wait()
		conn := ha.current()

		// Callbacks each run on their own goroutine, so concurrent service
		// calls are the normal case rather than an edge case.
		var wg sync.WaitGroup
		for range senders {
			wg.Add(1)
			go func() {
				defer wg.Done()
				assert.NoError(t, c.Send(mapRequest{"type": "call_service"}))
			}()
		}
		wg.Wait()
		synctest.Wait()

		seen := conn.requests()
		require.Len(t, seen, senders)

		// Home Assistant refuses any id that does not exceed the last one it
		// received, answering ERR_ID_REUSE and never running the request. An id
		// allocated atomically is not enough: it has to reach the socket in the
		// order it was taken.
		for i := 1; i < len(seen); i++ {
			assert.Greater(t, seen[i].ID, seen[i-1].ID,
				"request %d carried a smaller id than the one before it", i)
		}
	})
}

func TestClientSendReportsWriteFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		c := connectedClient(t, ha, Options{})
		synctest.Wait()

		require.NoError(t, c.Close())

		err := c.Send(mapRequest{"type": "call_service"})
		assert.Error(t, err)
	})
}
