package connect

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// awaitReconnect advances past the backoff delay and lets the new connection
// settle. synctest.Wait alone is not enough: a goroutine parked on the backoff
// timer counts as durably blocked, so Wait returns with the delay still
// outstanding and the client still holding the dead connection.
func awaitReconnect() {
	time.Sleep(5 * time.Second)
	synctest.Wait()
}

func TestClientReconnectsAfterServerDrop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		connectedClient(t, ha, Options{})
		synctest.Wait()
		require.Equal(t, 1, ha.dialCount())

		ha.current().serverClose()
		awaitReconnect()

		assert.Equal(t, 2, ha.dialCount(), "a dropped connection must be re-established")
	})
}

func TestClientReplaysSubscriptionsOnReconnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		c := connectedClient(t, ha, Options{})

		var delivered atomic.Int64
		require.NoError(t, c.Subscribe(Subscription{EventType: "state_changed"}, func(Message) {
			delivered.Add(1)
		}))
		synctest.Wait()

		first := ha.current()
		require.Len(t, first.subscriptions(), 1)

		first.serverClose()
		awaitReconnect()

		second := ha.current()
		require.NotSame(t, first, second)
		replayed := second.subscriptions()
		require.Len(t, replayed, 1, "the subscription must be re-sent on the new connection")

		// Ids belong to the connection that issued them, so the replay has to
		// allocate a new one rather than reuse the old.
		assert.NotEqual(t, first.subscriptions()[0], replayed[0])

		second.emit(replayed[0], "state_changed")
		synctest.Wait()
		assert.Equal(t, int64(1), delivered.Load(), "events must flow again after a reconnect")
	})
}

func TestClientDoesNotDuplicateSubscriptionsOnReconnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		c := connectedClient(t, ha, Options{})

		for _, eventType := range []string{"state_changed", "call_service"} {
			require.NoError(t, c.Subscribe(Subscription{EventType: eventType}, func(Message) {}))
		}
		synctest.Wait()

		ha.current().serverClose()
		awaitReconnect()

		assert.Len(t, ha.current().subscriptions(), 2,
			"each subscription must be replayed exactly once")
	})
}

func TestClientBacksOffBetweenFailedAttempts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		connectedClient(t, ha, Options{})
		synctest.Wait()

		// Refuse every dial from the reconnect onwards, so the client is forced
		// through several attempts.
		ha.failDialsFrom(2, context.DeadlineExceeded)

		start := time.Now()
		ha.current().serverClose()

		// Four refused attempts cost roughly 1+2+4+8 seconds before the base
		// delay is even reached again; sample partway through that sequence.
		time.Sleep(4 * time.Second)
		synctest.Wait()

		attempts := ha.dialCount()
		assert.GreaterOrEqual(t, attempts, 2, "it must keep retrying")
		assert.Less(t, attempts, 8,
			"delays must grow; a fixed retry would have burned far more attempts by now")
		assert.GreaterOrEqual(t, time.Since(start), 4*time.Second)

		ha.allowDials()
		time.Sleep(time.Minute)
		synctest.Wait()
		assert.Greater(t, ha.dialCount(), attempts, "it must recover once dials succeed again")
	})
}

func TestClientStopsRetryingWhenTheTokenIsRefused(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		c := connectedClient(t, ha, Options{})
		synctest.Wait()

		// The token is revoked while the app is running, so the reconnect
		// handshake will be rejected rather than the dial.
		ha.rotateToken("rotated-token")
		ha.current().serverClose()

		time.Sleep(5 * time.Minute)
		synctest.Wait()

		// Retrying a refused token only produces the same answer more slowly,
		// so the client gives up instead of reconnecting forever.
		assert.Equal(t, 2, ha.dialCount(), "a refused token must not be retried")

		select {
		case <-c.Done():
		default:
			t.Fatal("giving up must be observable, or the app waits on a dead client forever")
		}
	})
}

func TestClientDoneBlocksBeforeConnecting(t *testing.T) {
	c := newClientWithDialer(nil, testToken, Options{})
	select {
	case <-c.Done():
		t.Fatal("a client that never connected has not finished")
	default:
	}
}

func TestClientPingTimeoutForcesReconnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		connectedClient(t, ha, Options{
			PingInterval: time.Second,
			PingTimeout:  time.Second,
		})
		synctest.Wait()

		// The socket still accepts bytes, but nothing answers: the failure a
		// TCP-level check cannot see.
		ha.current().ignorePingsFrom()

		time.Sleep(10 * time.Second)
		synctest.Wait()

		assert.Greater(t, ha.dialCount(), 1, "an unanswered ping must drop the connection")
	})
}

func TestClientKeepalivePingsWhileIdle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		connectedClient(t, ha, Options{PingInterval: time.Second})
		synctest.Wait()

		conn := ha.current()
		time.Sleep(5 * time.Second)
		synctest.Wait()

		assert.GreaterOrEqual(t, conn.countOf(typePing), 4)
		assert.Equal(t, 1, ha.dialCount(), "answered pings must not disturb the connection")
	})
}
