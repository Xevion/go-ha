package connect

import (
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// OnEvent runs on the reader, so it must see every event in wire order, before
// the workers dispatch them out of order. It must also see an event the bounded
// queue then drops: the cache it maintains has to reflect the change even when
// the backlog means no handler will run for it.
func TestOnEventFiresInWireOrderIncludingDropped(t *testing.T) {
	var mu sync.Mutex
	var applied []int64

	c, err := NewClient(&url.URL{Scheme: "http", Host: "localhost:8123"}, "test-token", Options{
		QueueSize: 2,
		OnEvent: func(m Message) {
			mu.Lock()
			applied = append(applied, m.ID)
			mu.Unlock()
		},
	})
	require.NoError(t, err)

	// Nothing drains c.events here, so after QueueSize events the rest are
	// dropped. Every one must still have been applied, in the order it arrived.
	var rep dropReporter
	for id := int64(1); id <= 5; id++ {
		c.route(Message{Type: typeEvent, ID: id}, &rep)
	}

	assert.Equal(t, []int64{1, 2, 3, 4, 5}, applied, "every event applied, in order")
	assert.Equal(t, uint64(3), c.Dropped(), "the three past the queue size were shed")
}
