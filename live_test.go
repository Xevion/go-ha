//go:build live

// Package ha's live suite runs against a real Home Assistant instance rather
// than a fake. Everything else in this repository tests the library against
// hatest, which encodes this package's own beliefs about the protocol; a wrong
// belief is invisible there, because the fake shares it. These tests exist to
// catch exactly that class of error.
//
// Every assertion about an effect is made by reading Home Assistant's REST API
// directly, not through the library, so the verification path and the code
// under test do not share an implementation.
//
//	HA_URL=http://localhost:8123 HA_TOKEN=<long-lived> go test -tags live -v
package ha

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Xevion/go-ha/services"
	"github.com/Xevion/go-ha/types"
)

func liveEnv(t *testing.T) (url, token string) {
	t.Helper()
	url, token = os.Getenv("HA_URL"), os.Getenv("HA_TOKEN")
	if url == "" || token == "" {
		t.Skip("HA_URL and HA_TOKEN required for the live suite")
	}
	return url, token
}

// probe talks to Home Assistant over REST, independently of the library. It is
// the check side of every round trip: if both sides went through this package,
// a shared misunderstanding of the protocol would cancel out and pass.
type probe struct {
	t     *testing.T
	url   string
	token string
	c     *http.Client
}

func newProbe(t *testing.T) *probe {
	url, token := liveEnv(t)
	return &probe{t: t, url: url, token: token, c: &http.Client{Timeout: 10 * time.Second}}
}

func (p *probe) do(method, path string, body any) ([]byte, int) {
	p.t.Helper()

	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(p.t, err)
		rdr = strings.NewReader(string(raw))
	}

	req, err := http.NewRequest(method, p.url+path, rdr)
	require.NoError(p.t, err)
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.c.Do(req)
	require.NoError(p.t, err)
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	require.NoError(p.t, err)
	return out, resp.StatusCode
}

// toggleUntil drives an entity off and on repeatedly until fired reports
// progress, and returns whether it ever did.
//
// A single transition is not a sound probe after a restart. Home Assistant
// serving HTTP again does not mean this client has resubscribed, it may still
// be backing off, and a transition in that window is lost for good because
// Home Assistant replays nothing to a client that was not listening. Watching
// the cache instead does not help: the reseed on reconnect fills it from a
// snapshot, so it goes current whether or not any event was delivered.
//
// What can be asserted is recovery: once the client is back, transitions reach
// the automation again.
func toggleUntil(t *testing.T, p *probe, entityID string, progressed func() bool, within time.Duration) bool {
	t.Helper()

	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		p.set("input_boolean", "turn_off", entityID)
		p.set("input_boolean", "turn_on", entityID)

		settle := time.Now().Add(2 * time.Second)
		for time.Now().Before(settle) {
			if progressed() {
				return true
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return progressed()
}

// reachable reports whether Home Assistant is serving, tolerating the refused
// connections and resets that a restart produces. do is deliberately strict, so
// waiting for a restart needs its own path.
func (p *probe) reachable() bool {
	req, err := http.NewRequest(http.MethodGet, p.url+"/api/", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+p.token)

	resp, err := p.c.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

type probeState struct {
	EntityID   string         `json:"entity_id"`
	State      string         `json:"state"`
	Attributes map[string]any `json:"attributes"`
}

func (p *probe) state(entityID string) probeState {
	p.t.Helper()
	raw, code := p.do(http.MethodGet, "/api/states/"+entityID, nil)
	require.Equal(p.t, http.StatusOK, code, "reading %s: %s", entityID, raw)

	var s probeState
	require.NoError(p.t, json.Unmarshal(raw, &s))
	return s
}

// set drives an entity from Home Assistant's side, so a trigger under test is
// reacting to a change this package did not make.
func (p *probe) set(domain, service, entityID string) {
	p.t.Helper()
	raw, code := p.do(http.MethodPost, "/api/services/"+domain+"/"+service,
		map[string]any{"entity_id": entityID})
	require.Equal(p.t, http.StatusOK, code, "calling %s.%s: %s", domain, service, raw)
}

// awaitState polls until the entity reaches want, and fails describing what it
// actually held. Home Assistant applies a service call asynchronously, so the
// state immediately after one is not yet the state it produces.
func (p *probe) awaitState(entityID, want string, within time.Duration) {
	p.t.Helper()

	deadline := time.Now().Add(within)
	var last string
	for time.Now().Before(deadline) {
		last = p.state(entityID).State
		if last == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	p.t.Fatalf("%s was %q after %s, want %q", entityID, last, within, want)
}

// liveApp connects to the real instance and starts the app, returning once the
// entity cache has actually been seeded.
func liveApp(t *testing.T, automations ...Automation) *App {
	t.Helper()
	url, token := liveEnv(t)

	app, err := NewApp(types.NewAppRequest{URL: url, HAAuthToken: token})
	require.NoError(t, err, "connecting to Home Assistant")

	if len(automations) > 0 {
		require.NoError(t, app.RegisterAutomations(automations...))
	}

	done := make(chan error, 1)
	go func() { done <- app.Start() }()
	t.Cleanup(func() {
		require.NoError(t, app.Close())
		<-done
	})

	// Start returns only when the app stops, so readiness is the seeded cache.
	require.Eventually(t, func() bool {
		entities, err := app.State().ListEntities()
		return err == nil && len(entities) > 0
	}, 15*time.Second, 100*time.Millisecond, "cache never seeded from /api/states")

	return app
}

// fired records that an automation ran, and lets a test wait for it.
type fired struct {
	mu   sync.Mutex
	runs []Run
	ch   chan struct{}
}

func newFired() *fired { return &fired{ch: make(chan struct{}, 64)} }

func (f *fired) action() Action {
	return func(_ context.Context, run Run) error {
		f.mu.Lock()
		f.runs = append(f.runs, run)
		f.mu.Unlock()
		select {
		case f.ch <- struct{}{}:
		default:
		}
		return nil
	}
}

func (f *fired) await(t *testing.T, within time.Duration) Run {
	t.Helper()
	select {
	case <-f.ch:
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.runs[len(f.runs)-1]
	case <-time.After(within):
		t.Fatalf("automation did not fire within %s", within)
		return Run{}
	}
}

func (f *fired) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.runs)
}

// The cache is seeded from a real /api/states payload, so this checks that this
// package can decode what Home Assistant actually serves for every entity a
// stock install exposes, not just the handful the fake emits.
func TestLiveCacheSeedsFromRealStates(t *testing.T) {
	app := liveApp(t)
	p := newProbe(t)

	entities, err := app.State().ListEntities()
	require.NoError(t, err)

	raw, code := p.do(http.MethodGet, "/api/states", nil)
	require.Equal(t, http.StatusOK, code)
	var actual []probeState
	require.NoError(t, json.Unmarshal(raw, &actual))

	assert.Equal(t, len(actual), len(entities),
		"the cache should hold exactly what Home Assistant reports")

	sun, err := app.State().Get(SunEntityID)
	require.NoError(t, err, "sun.sun must be readable from the cache")
	assert.Equal(t, p.state(SunEntityID).State, sun.State)
}

// The sun migration rests entirely on attribute names and a timestamp format
// that were assumed, never observed. If Home Assistant renamed one or emitted a
// layout time.RFC3339 rejects, every sun automation silently stops firing.
func TestLiveSunAttributesParseAndAreInTheFuture(t *testing.T) {
	app := liveApp(t)
	p := newProbe(t)

	sun := p.state(SunEntityID)
	assert.Contains(t, []string{"above_horizon", "below_horizon"}, sun.State,
		"SunIsUp compares against the literal above_horizon")

	for _, tc := range []struct {
		event SunEvent
		attr  string
	}{
		{SunRising, "next_rising"},
		{SunSetting, "next_setting"},
		{SunDawn, "next_dawn"},
		{SunDusk, "next_dusk"},
	} {
		t.Run(tc.attr, func(t *testing.T) {
			raw, ok := sun.Attributes[tc.attr].(string)
			require.True(t, ok, "Home Assistant no longer publishes %s", tc.attr)

			parsed, err := time.Parse(time.RFC3339, raw)
			require.NoError(t, err, "%s = %q does not parse as RFC3339", tc.attr, raw)
			assert.True(t, parsed.After(time.Now()),
				"%s should be in the future, got %s", tc.attr, parsed)

			// The trigger has to reach the same instant through the cache.
			trig := newSunTrigger(tc.event, nil).(*sunTrigger)
			trig.bind(app.State())
			got, ok := trig.NextTime(time.Now())
			require.True(t, ok, "trigger could not derive a time from real sun.sun")
			assert.WithinDuration(t, parsed, got, time.Second)
		})
	}
}

// A sun trigger reaching an App must be bound to that App's reader by
// registration. Unbound, NextTime returns false forever and the schedule is
// silently dropped, which is how the original Put() bug behaved.
func TestLiveSunTriggerIsBoundByRegistration(t *testing.T) {
	f := newFired()
	app := liveApp(t, NewAutomation("sun-bound").
		On(Sunset(), Sunrise(), Dawn(), Dusk()).
		Do(f.action()).
		MustBuild())

	assert.Equal(t, 4, app.schedules.len(),
		"every sun trigger should have produced a scheduled entry")
}

// The round trip that matters: the library calls a service, and Home Assistant
// is asked over REST whether it actually happened.
func TestLiveServiceCallChangesRealState(t *testing.T) {
	app := liveApp(t)
	p := newProbe(t)

	p.set("input_boolean", "turn_off", "input_boolean.live_probe")
	p.awaitState("input_boolean.live_probe", "off", 5*time.Second)

	require.NoError(t, app.Services().InputBoolean.TurnOn("input_boolean.live_probe"))
	p.awaitState("input_boolean.live_probe", "on", 5*time.Second)

	require.NoError(t, app.Services().InputBoolean.TurnOff("input_boolean.live_probe"))
	p.awaitState("input_boolean.live_probe", "off", 5*time.Second)
}

// Service data has to survive the request shaping and be understood by Home
// Assistant. The shape tests assert what this package sends; only Home
// Assistant can say whether it acted on it.
func TestLiveServiceDataIsApplied(t *testing.T) {
	app := liveApp(t)
	p := newProbe(t)

	require.NoError(t, app.Services().Light.TurnOn("light.bed_light",
		map[string]any{"brightness": 137}))

	require.Eventually(t, func() bool {
		s := p.state("light.bed_light")
		b, ok := s.Attributes["brightness"].(float64)
		return s.State == "on" && ok && int(b) == 137
	}, 5*time.Second, 100*time.Millisecond,
		"brightness was not applied, got %v", p.state("light.bed_light").Attributes["brightness"])
}

// Zero is a real setpoint, and the pointer fields exist so it is distinguishable
// from unset. This proves the distinction survives to Home Assistant.
func TestLiveClimateSetpointRoundTrips(t *testing.T) {
	app := liveApp(t)
	p := newProbe(t)

	require.NoError(t, app.Services().Climate.SetTemperature("climate.heatpump",
		types.SetTemperatureRequest{Temperature: types.Ptr(float32(23))}))

	require.Eventually(t, func() bool {
		temp, ok := p.state("climate.heatpump").Attributes["temperature"].(float64)
		return ok && temp == 23
	}, 5*time.Second, 100*time.Millisecond, "setpoint did not reach Home Assistant")
}

// Call is the escape hatch for services this package does not model. It has
// only ever been checked against a recorder that accepts anything.
func TestLiveEscapeHatchReachesAnUnmodelledService(t *testing.T) {
	app := liveApp(t)
	p := newProbe(t)

	require.NoError(t, services.Call(app.client, "input_number", "set_value",
		"input_number.live_level", map[string]any{"value": 42}))

	require.Eventually(t, func() bool {
		return p.state("input_number.live_level").State == "42.0"
	}, 5*time.Second, 100*time.Millisecond, "input_number did not take the value")
}

// An event trigger firing on a change this package did not make. Home Assistant
// drives the entity; the library must observe it over the real event stream.
func TestLiveEventTriggerObservesExternalChange(t *testing.T) {
	p := newProbe(t)
	p.set("input_boolean", "turn_off", "input_boolean.porch_motion")
	p.awaitState("input_boolean.porch_motion", "off", 5*time.Second)

	f := newFired()
	liveApp(t, NewAutomation("porch").
		On(StateChanged("input_boolean.porch_motion").To("on")).
		Do(f.action()).
		MustBuild())

	p.set("input_boolean", "turn_on", "input_boolean.porch_motion")

	run := f.await(t, 10*time.Second)
	assert.Equal(t, "input_boolean.porch_motion", run.Event.EntityID)
	assert.Equal(t, "on", run.Event.To.State)
	assert.Equal(t, "off", run.Event.From.State)
}

// Conditions read the cache, which is fed by the real stream. A condition
// evaluated against a state Home Assistant has since changed would fire the
// wrong way round.
func TestLiveConditionSeesTheCurrentRealState(t *testing.T) {
	p := newProbe(t)
	p.set("input_boolean", "turn_off", "input_boolean.live_probe")
	p.awaitState("input_boolean.live_probe", "off", 5*time.Second)

	f := newFired()
	liveApp(t, NewAutomation("gated").
		On(StateChanged("input_boolean.porch_motion")).
		When(StateIs("input_boolean.live_probe", "on")).
		Do(f.action()).
		MustBuild())

	// Gate closed: the trigger fires, the condition should refuse it.
	p.set("input_boolean", "turn_on", "input_boolean.porch_motion")
	time.Sleep(2 * time.Second)
	assert.Zero(t, f.count(), "condition should have blocked the run while the gate was off")

	// Open the gate and let the cache observe it before triggering again.
	p.set("input_boolean", "turn_on", "input_boolean.live_probe")
	p.awaitState("input_boolean.live_probe", "on", 5*time.Second)
	time.Sleep(500 * time.Millisecond)

	p.set("input_boolean", "turn_off", "input_boolean.porch_motion")
	f.await(t, 10*time.Second)
}

// For waits out a hold against real events. A change away before the hold
// elapses has to cancel it.
func TestLiveForFiresOnlyAfterTheHoldElapses(t *testing.T) {
	p := newProbe(t)
	p.set("input_boolean", "turn_off", "input_boolean.hold_test")
	p.awaitState("input_boolean.hold_test", "off", 5*time.Second)

	f := newFired()
	liveApp(t, NewAutomation("held").
		On(StateChanged("input_boolean.hold_test").To("on").For(3*time.Second)).
		Do(f.action()).
		MustBuild())

	// Cancelled: turned back off before the hold elapses.
	p.set("input_boolean", "turn_on", "input_boolean.hold_test")
	time.Sleep(1 * time.Second)
	p.set("input_boolean", "turn_off", "input_boolean.hold_test")
	time.Sleep(4 * time.Second)
	assert.Zero(t, f.count(), "a change away from the held state must cancel the wait")

	// Sustained: left on past the hold.
	p.set("input_boolean", "turn_on", "input_boolean.hold_test")
	f.await(t, 10*time.Second)
}

// Schedules run off the same loop as everything else while a real connection is
// live. This is the cheapest check that the run loop is not wedged.
func TestLiveIntervalFiresWhileConnected(t *testing.T) {
	f := newFired()
	liveApp(t, NewAutomation("ticker").
		On(Every(2*time.Second)).
		Do(f.action()).
		MustBuild())

	f.await(t, 15*time.Second)
}

// An automation acting on the state that triggered it, which is the shape of
// almost every real automation: observe a change, read state, call a service,
// and have Home Assistant reflect it.
func TestLiveEndToEndAutomationLoop(t *testing.T) {
	p := newProbe(t)
	p.set("input_boolean", "turn_off", "input_boolean.porch_motion")
	p.set("light", "turn_off", "light.ceiling_lights")
	p.awaitState("input_boolean.porch_motion", "off", 5*time.Second)
	p.awaitState("light.ceiling_lights", "off", 5*time.Second)

	f := newFired()
	liveApp(t, NewAutomation("motion-light").
		On(StateChanged("input_boolean.porch_motion").To("on")).
		When(StateIsNot("light.ceiling_lights", "on")).
		Do(func(ctx context.Context, run Run) error {
			if err := run.Services.Light.TurnOn("light.ceiling_lights",
				map[string]any{"brightness": 200}); err != nil {
				return err
			}
			return f.action()(ctx, run)
		}).
		MustBuild())

	p.set("input_boolean", "turn_on", "input_boolean.porch_motion")
	f.await(t, 10*time.Second)

	require.Eventually(t, func() bool {
		s := p.state("light.ceiling_lights")
		b, ok := s.Attributes["brightness"].(float64)
		return s.State == "on" && ok && int(b) == 200
	}, 5*time.Second, 100*time.Millisecond, "the automation's service call did not land")
}

// Reconnection is the least testable thing against a fake, which always
// reconnects cleanly, and the most consequential in production: a lost
// subscription leaves the app running and permanently deaf.
func TestLiveReconnectResubscribesAfterHomeAssistantRestarts(t *testing.T) {
	if os.Getenv("HA_CONTAINER") == "" {
		t.Skip("HA_CONTAINER required to restart Home Assistant")
	}

	p := newProbe(t)
	p.set("input_boolean", "turn_off", "input_boolean.porch_motion")
	p.awaitState("input_boolean.porch_motion", "off", 5*time.Second)

	f := newFired()
	liveApp(t, NewAutomation("survives-restart").
		On(StateChanged("input_boolean.porch_motion").To("on")).
		Do(f.action()).
		MustBuild())

	// Prove the subscription works before the outage.
	p.set("input_boolean", "turn_on", "input_boolean.porch_motion")
	f.await(t, 10*time.Second)
	before := f.count()

	restartHA(t)

	recovered := toggleUntil(t, p, "input_boolean.porch_motion",
		func() bool { return f.count() > before }, 90*time.Second)
	require.True(t, recovered,
		"the automation never fired again after the restart; the subscription was not replayed")
}

// The cache has to be reseeded on reconnect, not merely kept. Anything that
// changed during the outage was never delivered as an event.
func TestLiveCacheReseedsAfterRestart(t *testing.T) {
	if os.Getenv("HA_CONTAINER") == "" {
		t.Skip("HA_CONTAINER required to restart Home Assistant")
	}

	p := newProbe(t)
	p.set("input_boolean", "turn_off", "input_boolean.live_probe")
	p.awaitState("input_boolean.live_probe", "off", 5*time.Second)

	app := liveApp(t)
	require.Eventually(t, func() bool {
		s, err := app.State().Get("input_boolean.live_probe")
		return err == nil && s.State == "off"
	}, 10*time.Second, 100*time.Millisecond)

	restartHA(t)

	// input_boolean restores its previous state across a restart, so drive it
	// after Home Assistant is back and confirm the cache tracks it again.
	p.awaitState("input_boolean.live_probe", "off", 60*time.Second)
	p.set("input_boolean", "turn_on", "input_boolean.live_probe")
	p.awaitState("input_boolean.live_probe", "on", 30*time.Second)

	require.Eventually(t, func() bool {
		s, err := app.State().Get("input_boolean.live_probe")
		return err == nil && s.State == "on"
	}, 60*time.Second, 250*time.Millisecond,
		"the cache did not recover after the connection dropped")
}

// Pins what a service call actually reports, against a real refusal. The fake
// answers success unconditionally, so none of this was observable before.
//
// Two things are true and neither is obvious from the signature: Home Assistant
// accepts a call naming an entity that does not exist, and a call it does
// refuse still returns nil, because the service methods write and return
// without waiting for the answer.
func TestLiveServiceCallErrorReporting(t *testing.T) {
	app := liveApp(t)

	t.Run("a missing entity is not an error to Home Assistant", func(t *testing.T) {
		assert.NoError(t, services.Call(app.client, "input_boolean", "turn_on",
			"input_boolean.does_not_exist", nil))
	})

	t.Run("a refused call still returns nil through the service API", func(t *testing.T) {
		assert.NoError(t, services.Call(app.client, "nonexistent_domain", "nope", "x.y", nil),
			"Send does not wait for the answer, so the refusal is only logged")
	})

	// The correlation added for the transport does surface it, which is what
	// makes the fire-and-forget choice a choice rather than a limitation.
	t.Run("the synchronous path surfaces the refusal", func(t *testing.T) {
		req := services.NewBaseServiceRequest("x.y")
		req.Domain = "nonexistent_domain"
		req.Service = "nope"

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := app.client.Call(ctx, &req)
		require.Error(t, err, "Home Assistant refuses an unknown service")
		assert.Contains(t, err.Error(), "not_found")
	})
}

// Reading an entity that does not exist has to be distinguishable from an
// entity that is merely off.
func TestLiveUnknownEntityIsReportedMissing(t *testing.T) {
	app := liveApp(t)

	_, err := app.State().Get("light.definitely_not_real")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEntityNotFound)
}

func restartHA(t *testing.T) {
	t.Helper()
	name := os.Getenv("HA_CONTAINER")

	out, err := exec.Command("docker", "restart", name).CombinedOutput()
	require.NoError(t, err, "restarting %s: %s", name, out)

	p := newProbe(t)

	// Wait for it to actually go down first. Restart is not instant, and polling
	// straight through a still-serving old process reports ready immediately.
	downBy := time.Now().Add(30 * time.Second)
	for p.reachable() && time.Now().Before(downBy) {
		time.Sleep(200 * time.Millisecond)
	}

	upBy := time.Now().Add(120 * time.Second)
	for time.Now().Before(upBy) {
		if p.reachable() {
			// Serving the API does not mean the entity registry is populated.
			time.Sleep(2 * time.Second)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("Home Assistant did not come back after the restart")
}
