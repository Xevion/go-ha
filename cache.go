package ha

import "sync"

// entityCache holds the last known state of every entity, seeded from a REST
// snapshot and maintained from the event stream. Conditions read from here
// rather than issuing an HTTP request per check.
type entityCache struct {
	mu       sync.RWMutex
	entities map[string]EntityState

	// touched names the entities the stream wrote while a snapshot was in
	// flight. Those writes are newer than the snapshot, which was requested
	// before them.
	touched map[string]struct{}
	pending bool
	seeded  bool
}

func newEntityCache() *entityCache {
	return &entityCache{entities: map[string]EntityState{}}
}

// beginSeed opens a snapshot window. It must be called before the request that
// produces the snapshot, so racing events can be recognised.
func (c *entityCache) beginSeed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.touched = map[string]struct{}{}
	c.pending = true
}

// finishSeed installs a snapshot, keeping any entity the stream updated while
// it was in flight. Entities missing from it are dropped: they no longer exist.
func (c *entityCache) finishSeed(list []EntityState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	next := make(map[string]EntityState, len(list))
	for _, es := range list {
		next[es.EntityID] = es
	}
	for id := range c.touched {
		if es, ok := c.entities[id]; ok {
			next[id] = es
		}
	}

	c.entities = next
	c.touched = nil
	c.pending = false
	c.seeded = true
}

func (c *entityCache) apply(es EntityState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entities[es.EntityID] = es
	if c.pending {
		c.touched[es.EntityID] = struct{}{}
	}
}

// remove forgets an entity that Home Assistant deleted, reported as a state
// change to a null new state.
func (c *entityCache) remove(entityId string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entities, entityId)
	if c.pending {
		c.touched[entityId] = struct{}{}
	}
}

func (c *entityCache) get(entityId string) (EntityState, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	es, ok := c.entities[entityId]
	return es, ok
}

// ready reports whether a snapshot has landed. Until one has, the cache cannot
// distinguish an unknown entity from one it has simply not seen yet.
func (c *entityCache) ready() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.seeded
}

func (c *entityCache) list() []EntityState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]EntityState, 0, len(c.entities))
	for _, es := range c.entities {
		out = append(out, es)
	}
	return out
}
