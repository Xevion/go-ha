package ha

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dromara/carbon/v2"

	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/types"
)

type StateReader interface {
	AfterSunrise(...types.DurationString) bool
	BeforeSunrise(...types.DurationString) bool
	AfterSunset(...types.DurationString) bool
	BeforeSunset(...types.DurationString) bool
	ListEntities() ([]EntityState, error)
	Get(entityId string) (EntityState, error)
	Equals(entityId, state string) (bool, error)
}

// state is used to retrieve state from Home Assistant.
type state struct {
	httpClient *internal.HttpClient
	cache      *entityCache
	latitude   float64
	longitude  float64
}

type EntityState struct {
	EntityID    string         `json:"entity_id"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
	LastChanged time.Time      `json:"last_changed"`
}

func newState(c *internal.HttpClient, homeZoneEntityId string) (*state, error) {
	s := &state{httpClient: c, cache: newEntityCache()}

	// Ensure the zone exists and has required attributes
	entity, err := s.Get(homeZoneEntityId)
	if err != nil {
		return nil, fmt.Errorf("home zone entity '%s' not found: %w", homeZoneEntityId, err)
	}

	// Ensure it's a zone entity
	if !strings.HasPrefix(homeZoneEntityId, "zone.") {
		return nil, fmt.Errorf("entity '%s' is not a zone entity (must start with zone.)", homeZoneEntityId)
	}

	// Verify and extract latitude and longitude
	if entity.Attributes == nil {
		return nil, fmt.Errorf("home zone entity '%s' has no attributes", homeZoneEntityId)
	}

	if lat, ok := entity.Attributes["latitude"].(float64); ok {
		s.latitude = lat
	} else {
		return nil, fmt.Errorf("home zone entity '%s' missing valid latitude attribute", homeZoneEntityId)
	}

	if long, ok := entity.Attributes["longitude"].(float64); ok {
		s.longitude = long
	} else {
		return nil, fmt.Errorf("home zone entity '%s' missing valid longitude attribute", homeZoneEntityId)
	}

	return s, nil
}

// seed replaces the cache with a fresh snapshot of every entity. The window
// opens before the request so events that race it are not overwritten.
func (s *state) seed() error {
	s.cache.beginSeed()

	resp, err := s.httpClient.GetStates()
	if err != nil {
		return err
	}
	var list []EntityState
	if err := json.Unmarshal(resp, &list); err != nil {
		return fmt.Errorf("decoding state snapshot: %w", err)
	}

	s.cache.finishSeed(list)
	return nil
}

func (s *state) Get(entityId string) (EntityState, error) {
	if es, ok := s.cache.get(entityId); ok {
		return es, nil
	}
	if s.cache.ready() {
		return EntityState{}, fmt.Errorf("%w: %s", internal.ErrEntityNotFound, entityId)
	}

	resp, err := s.httpClient.GetState(entityId)
	if err != nil {
		return EntityState{}, err
	}
	es := EntityState{}
	err = json.Unmarshal(resp, &es)
	return es, err
}

// ListEntities returns a list of all entities in Home Assistant.
// See REST documentation for more details: https://developers.home-assistant.io/docs/api/rest/#actions
func (s *state) ListEntities() ([]EntityState, error) {
	if s.cache.ready() {
		return s.cache.list(), nil
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

func (s *state) BeforeSunrise(offset ...types.DurationString) bool {
	sunrise := getSunriseSunset(s, true, carbon.Now(), offset...)
	return carbon.Now().Lt(sunrise)
}

func (s *state) AfterSunrise(offset ...types.DurationString) bool {
	return !s.BeforeSunrise(offset...)
}

func (s *state) BeforeSunset(offset ...types.DurationString) bool {
	sunset := getSunriseSunset(s, false, carbon.Now(), offset...)
	return carbon.Now().Lt(sunset)
}

func (s *state) AfterSunset(offset ...types.DurationString) bool {
	return !s.BeforeSunset(offset...)
}
