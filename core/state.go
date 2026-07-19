package core

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Xevion/go-ha/internal"
)

type StateReader interface {
	ListEntities() ([]EntityState, error)
	Get(entityId string) (EntityState, error)
	Equals(entityId, state string) (bool, error)
}

// state is used to retrieve state from Home Assistant.
type state struct {
	httpClient *internal.HttpClient
	cache      *entityCache

	// seedMu serialises snapshot fetches against each other.
	seedMu sync.Mutex
}

type EntityState struct {
	EntityID   string         `json:"entity_id"`
	State      string         `json:"state"`
	Attributes map[string]any `json:"attributes"`

	// LastChanged moves only when the state itself changes.
	LastChanged time.Time `json:"last_changed"`

	// LastUpdated moves on any change, attributes included, which is what
	// orders two updates to the same entity.
	LastUpdated time.Time `json:"last_updated"`
}

// newState builds a reader backed by an empty cache. It fills on the first
// connection, when the snapshot is fetched.
func newState(c *internal.HttpClient) *state {
	return &state{httpClient: c, cache: newEntityCache()}
}

// seed replaces the cache with a fresh snapshot of every entity. The window
// opens before the request so events that race it are not overwritten.
//
// Seeds are serialised. Two reconnects in quick succession each fetch a
// snapshot, and overlapping them lets the older response install last and
// discards the newer window's events along with it.
func (s *state) seed() error {
	s.seedMu.Lock()
	defer s.seedMu.Unlock()

	s.cache.beginSeed()

	resp, err := s.httpClient.GetStates()
	if err != nil {
		s.cache.abandonSeed()
		return err
	}
	var list []EntityState
	if err := json.Unmarshal(resp, &list); err != nil {
		s.cache.abandonSeed()
		return fmt.Errorf("decoding state snapshot: %w", err)
	}

	s.cache.finishSeed(list)
	return nil
}

// applyEvent folds a state_changed event into the cache. A null new state means
// the entity was deleted.
func (s *state) applyEvent(raw []byte) {
	ev := parseEvent(raw)
	if ev.Type != eventStateChanged || ev.EntityID == "" {
		return
	}

	if ev.Deleted {
		s.cache.remove(ev.EntityID)
		return
	}
	s.cache.apply(ev.To)
}

func (s *state) Get(entityId string) (EntityState, error) {
	es, found, seeded := s.cache.lookup(entityId)
	if found {
		return es, nil
	}
	if seeded {
		return EntityState{}, fmt.Errorf("%w: %s", internal.ErrEntityNotFound, entityId)
	}

	resp, err := s.httpClient.GetState(entityId)
	if err != nil {
		return EntityState{}, err
	}
	err = json.Unmarshal(resp, &es)
	return es, err
}

// ListEntities returns a list of all entities in Home Assistant.
// See REST documentation for more details: https://developers.home-assistant.io/docs/api/rest/#actions
func (s *state) ListEntities() ([]EntityState, error) {
	if entities, seeded := s.cache.snapshot(); seeded {
		return entities, nil
	}

	resp, err := s.httpClient.GetStates()
	if err != nil {
		return nil, err
	}
	es := []EntityState{}
	err = json.Unmarshal(resp, &es)
	return es, err
}

func (s *state) Equals(entityId string, expectedState string) (bool, error) {
	currentState, err := s.Get(entityId)
	if err != nil {
		return false, err
	}
	return currentState.State == expectedState, nil
}
