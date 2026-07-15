package ha

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Xevion/go-ha/internal"
)

// stateWithServer builds a state backed by a stub Home Assistant REST API. The
// counter reports how many requests it served.
func stateWithServer(t *testing.T, handler http.HandlerFunc) (*state, *int) {
	t.Helper()

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	return &state{
		httpClient: internal.NewHttpClient(context.Background(), u, "token"),
		cache:      newEntityCache(),
	}, &calls
}

func TestGetServesFromCacheWithoutHTTP(t *testing.T) {
	s, calls := stateWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("cache hit must not reach Home Assistant")
	})

	s.cache.beginSeed()
	s.cache.finishSeed([]EntityState{entity("light.kitchen", "on")})

	got, err := s.Get("light.kitchen")
	require.NoError(t, err)
	assert.Equal(t, "on", got.State)
	assert.Zero(t, *calls)
}

func TestGetFallsBackToHTTPBeforeTheCacheIsSeeded(t *testing.T) {
	s, calls := stateWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"entity_id":"light.kitchen","state":"on"}`))
	})

	got, err := s.Get("light.kitchen")
	require.NoError(t, err)
	assert.Equal(t, "on", got.State)
	assert.Equal(t, 1, *calls, "an unseeded cache cannot answer, so the request goes out")
}

func TestGetReportsNotFoundOnceSeeded(t *testing.T) {
	s, calls := stateWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("a seeded cache knows the entity does not exist")
	})

	s.cache.beginSeed()
	s.cache.finishSeed([]EntityState{entity("light.kitchen", "on")})

	_, err := s.Get("light.missing")
	assert.ErrorIs(t, err, internal.ErrEntityNotFound)
	assert.Zero(t, *calls)
}

func TestSeedPopulatesTheCache(t *testing.T) {
	s, _ := stateWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"entity_id":"light.kitchen","state":"on"},
		                 {"entity_id":"light.hall","state":"off"}]`))
	})

	require.NoError(t, s.seed())

	require.True(t, s.cache.ready())
	got, ok := s.cache.get("light.hall")
	require.True(t, ok)
	assert.Equal(t, "off", got.State)
}

func TestSeedLeavesTheCacheUnseededOnFailure(t *testing.T) {
	// Unauthorized rather than a 5xx: the client retries 5xx with real backoff,
	// which this test has no reason to wait through.
	s, _ := stateWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	err := s.seed()
	require.Error(t, err)
	assert.False(t, s.cache.ready(), "a failed snapshot must not be mistaken for an empty one")
}

func TestEqualsUsesTheCache(t *testing.T) {
	s, calls := stateWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("Equals runs per condition check and must not issue a request")
	})

	s.cache.beginSeed()
	s.cache.finishSeed([]EntityState{entity("light.kitchen", "on")})

	match, err := s.Equals("light.kitchen", "on")
	require.NoError(t, err)
	assert.True(t, match)
	assert.Zero(t, *calls)
}

func TestListEntitiesServesFromCache(t *testing.T) {
	s, calls := stateWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("a seeded cache already holds every entity")
	})

	s.cache.beginSeed()
	s.cache.finishSeed([]EntityState{entity("light.kitchen", "on"), entity("light.hall", "off")})

	list, err := s.ListEntities()
	require.NoError(t, err)
	assert.Len(t, list, 2)
	assert.Zero(t, *calls)
}

func TestGetPropagatesHTTPErrors(t *testing.T) {
	s, _ := stateWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := s.Get("light.missing")
	assert.True(t, errors.Is(err, internal.ErrEntityNotFound))
}

func TestApplyEventUpdatesTheCache(t *testing.T) {
	s, _ := stateWithServer(t, func(w http.ResponseWriter, r *http.Request) {})
	s.cache.beginSeed()
	s.cache.finishSeed([]EntityState{entity("light.kitchen", "on")})

	s.applyEvent([]byte(`{"type":"event","event":{"event_type":"state_changed","data":{
		"entity_id":"light.kitchen",
		"old_state":{"entity_id":"light.kitchen","state":"on"},
		"new_state":{"entity_id":"light.kitchen","state":"off","attributes":{"brightness":12}}}}}`))

	got, ok := s.cache.get("light.kitchen")
	require.True(t, ok)
	assert.Equal(t, "off", got.State)
	assert.Equal(t, float64(12), got.Attributes["brightness"])
}

func TestApplyEventForgetsDeletedEntities(t *testing.T) {
	s, _ := stateWithServer(t, func(w http.ResponseWriter, r *http.Request) {})
	s.cache.beginSeed()
	s.cache.finishSeed([]EntityState{entity("light.kitchen", "on")})

	// Home Assistant reports a removed entity as a change to a null new state.
	s.applyEvent([]byte(`{"type":"event","event":{"event_type":"state_changed","data":{
		"entity_id":"light.kitchen",
		"old_state":{"entity_id":"light.kitchen","state":"on"},
		"new_state":null}}}`))

	_, ok := s.cache.get("light.kitchen")
	assert.False(t, ok, "a deleted entity must not linger in the cache")
}

func TestApplyEventIgnoresMalformedMessages(t *testing.T) {
	s, _ := stateWithServer(t, func(w http.ResponseWriter, r *http.Request) {})
	s.cache.beginSeed()
	s.cache.finishSeed([]EntityState{entity("light.kitchen", "on")})

	s.applyEvent([]byte(`not json`))
	s.applyEvent([]byte(`{"event":{"data":{}}}`))

	got, ok := s.cache.get("light.kitchen")
	require.True(t, ok, "a malformed event must not disturb known state")
	assert.Equal(t, "on", got.State)
}
