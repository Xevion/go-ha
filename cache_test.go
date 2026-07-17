package ha

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func entity(id, s string) EntityState {
	return EntityState{EntityID: id, State: s, LastChanged: time.Unix(0, 0)}
}

func TestCacheStartsEmptyAndUnseeded(t *testing.T) {
	c := newEntityCache()

	assert.False(t, c.ready())
	_, ok := c.get("light.kitchen")
	assert.False(t, ok)
}

func TestCacheServesSeededEntities(t *testing.T) {
	c := newEntityCache()

	c.beginSeed()
	c.finishSeed([]EntityState{entity("light.kitchen", "on")})

	require.True(t, c.ready())
	got, ok := c.get("light.kitchen")
	require.True(t, ok)
	assert.Equal(t, "on", got.State)
}

func TestCacheAppliesEvents(t *testing.T) {
	c := newEntityCache()
	c.beginSeed()
	c.finishSeed([]EntityState{entity("light.kitchen", "on")})

	c.apply(entity("light.kitchen", "off"))

	got, _ := c.get("light.kitchen")
	assert.Equal(t, "off", got.State)
}

// A snapshot is requested before it arrives, so events that land while it is in
// flight describe a world newer than the snapshot does. Letting the snapshot
// land on top would roll those entities backwards.
func TestSnapshotDoesNotOverwriteEventsThatRacedIt(t *testing.T) {
	c := newEntityCache()

	c.beginSeed()
	c.apply(entity("light.kitchen", "off"))
	c.finishSeed([]EntityState{
		entity("light.kitchen", "on"),
		entity("light.hall", "on"),
	})

	kitchen, _ := c.get("light.kitchen")
	assert.Equal(t, "off", kitchen.State, "an in-flight snapshot must not undo a newer event")

	hall, _ := c.get("light.hall")
	assert.Equal(t, "on", hall.State, "entities the stream did not touch still come from the snapshot")
}

// Reconnecting re-seeds, because events that happened during the outage were
// never delivered. The second snapshot has to be allowed to win.
func TestReseedReplacesStaleEntities(t *testing.T) {
	c := newEntityCache()
	c.beginSeed()
	c.finishSeed([]EntityState{entity("light.kitchen", "on")})
	c.apply(entity("light.kitchen", "off"))

	c.beginSeed()
	c.finishSeed([]EntityState{entity("light.kitchen", "on")})

	got, _ := c.get("light.kitchen")
	assert.Equal(t, "on", got.State, "a fresh snapshot must overwrite state from before the outage")
}

func TestReseedDropsEntitiesThatDisappeared(t *testing.T) {
	c := newEntityCache()
	c.beginSeed()
	c.finishSeed([]EntityState{entity("light.kitchen", "on"), entity("light.gone", "on")})

	c.beginSeed()
	c.finishSeed([]EntityState{entity("light.kitchen", "on")})

	_, ok := c.get("light.gone")
	assert.False(t, ok, "an entity absent from the new snapshot was removed in Home Assistant")
}

func TestCacheListsEntities(t *testing.T) {
	c := newEntityCache()
	c.beginSeed()
	c.finishSeed([]EntityState{entity("light.kitchen", "on"), entity("light.hall", "off")})

	assert.Len(t, c.list(), 2)
}

// Events are dispatched by a pool of workers, so two updates to one entity can
// be applied in either order. The later one has to win regardless of which
// worker gets there first, or the cache is left permanently stale.
func TestApplyIgnoresAnUpdateOlderThanTheStoredOne(t *testing.T) {
	c := newEntityCache()
	c.beginSeed()
	c.finishSeed(nil)

	at := func(s string, secs int) EntityState {
		return EntityState{
			EntityID:    "light.kitchen",
			State:       s,
			LastUpdated: time.Date(2026, 7, 19, 3, 0, secs, 0, time.UTC),
		}
	}

	c.apply(at("on", 30))
	c.apply(at("off", 10))

	got, _ := c.get("light.kitchen")
	assert.Equal(t, "on", got.State, "an update that overtook a newer one must not win")
}

func TestApplyAcceptsUpdatesWithoutTimestamps(t *testing.T) {
	c := newEntityCache()
	c.beginSeed()
	c.finishSeed(nil)

	c.apply(entity("light.kitchen", "on"))
	c.apply(entity("light.kitchen", "off"))

	got, _ := c.get("light.kitchen")
	assert.Equal(t, "off", got.State, "without timestamps there is no ordering to enforce")
}
