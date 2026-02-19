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

func TestClientShedsEventsWhenTheQueueIsFull(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const emitted = 50

		ha := newFakeHA(t, testToken)
		c := connectedClient(t, ha, Options{QueueSize: 2, Workers: 1})

		release := make(chan struct{})
		var handled atomic.Int64
		require.NoError(t, c.Subscribe(Subscription{EventType: "state_changed"}, func(Message) {
			<-release
			handled.Add(1)
		}))
		synctest.Wait()

		conn := ha.current()
		subID := conn.subscriptions()[0]
		for range emitted {
			conn.emit(subID, "state_changed")
		}
		synctest.Wait()

		require.Greater(t, c.Dropped(), uint64(0), "a queue this small must overflow")

		// The old listener closed its channel and returned the moment a
		// non-blocking send failed, ending the process over a transient burst.
		assert.Equal(t, 1, ha.dialCount(), "shedding load must not cost the connection")

		close(release)
		synctest.Wait()

		// Every event is either handled or counted as dropped. Losing one
		// without recording it is the failure worth guarding against.
		assert.Equal(t, int64(emitted), handled.Load()+int64(c.Dropped()))

		_, err := c.Call(context.Background(), mapRequest{"type": typePing})
		assert.NoError(t, err, "the client must still be usable after shedding")
	})
}

func TestClientReaderKeepsRunningWhileHandlersBlock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		c := connectedClient(t, ha, Options{
			QueueSize:    1,
			Workers:      1,
			PingInterval: time.Second,
		})

		release := make(chan struct{})
		require.NoError(t, c.Subscribe(Subscription{EventType: "state_changed"}, func(Message) {
			<-release
		}))
		synctest.Wait()

		conn := ha.current()
		subID := conn.subscriptions()[0]
		for range 10 {
			conn.emit(subID, "state_changed")
		}

		// With the only worker wedged, pings still have to be answered: Home
		// Assistant hangs up on a client that stops reading for five seconds,
		// so the reader cannot be allowed to wait on handler progress.
		time.Sleep(5 * time.Second)
		synctest.Wait()

		assert.GreaterOrEqual(t, conn.countOf(typePing), 4)
		assert.Equal(t, 1, ha.dialCount())

		close(release)
		synctest.Wait()
	})
}

func TestClientDroppedStartsAtZero(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ha := newFakeHA(t, testToken)
		c := connectedClient(t, ha, Options{})

		var seen atomic.Int64
		require.NoError(t, c.Subscribe(Subscription{EventType: "state_changed"}, func(Message) {
			seen.Add(1)
		}))
		synctest.Wait()

		conn := ha.current()
		subID := conn.subscriptions()[0]
		for range 20 {
			conn.emit(subID, "state_changed")
		}
		synctest.Wait()

		assert.Equal(t, int64(20), seen.Load())
		assert.Zero(t, c.Dropped(), "a queue at its default size must absorb an ordinary burst")
	})
}
