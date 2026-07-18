# go-ha

Write strongly typed [Home Assistant](https://www.home-assistant.io/) automations in Go.

```bash
go get github.com/Xevion/go-ha
```

## An automation

```go
package main

import (
	"context"
	"log"
	"os"
	"time"

	ha "github.com/Xevion/go-ha"
	"github.com/Xevion/go-ha/types"
)

func main() {
	app, err := ha.NewApp(types.NewAppRequest{
		URL:         "http://192.168.1.123:8123",
		HAAuthToken: os.Getenv("HA_AUTH_TOKEN"),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()

	err = app.RegisterAutomations(
		ha.NewAutomation("hall light on motion").
			On(ha.StateChanged("binary_sensor.hall_motion").To("on")).
			When(ha.SunIsDown()).
			Throttle(time.Minute).
			Do(func(ctx context.Context, run ha.Run) error {
				return run.Services.Light.TurnOn("light.hall")
			}).
			MustBuild(),
	)
	if err != nil {
		log.Fatal(err)
	}

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
```

## The four layers

Every automation is a trigger, some conditions, a policy and an action.

| Layer | What it decides | Built with |
| --- | --- | --- |
| **Trigger** | when to consider running | `StateChanged`, `EventFired`, `Daily`, `Every`, `Cron`, `Sunrise`, `Sunset`, `Dawn`, `Dusk`, `AtStartup` |
| **Condition** | whether to go ahead | `StateIs`, `StateIsOneOf`, `TimeBetween`, `OnWeekdays`, `SunIsUp`, composed with `All`, `Any`, `Not` |
| **Policy** | what to do about overlap | `Mode`, `Throttle`, `Limit` |
| **Action** | the work | `Do(func(ctx, run) error)` |

### Triggers

Triggers come in two families and an automation can hold a mix of both, so one
rule can say "at sunset, or when the door opens":

```go
On(ha.Sunset(-15*time.Minute), ha.StateChanged("binary_sensor.door").To("on"))
```

Schedule triggers are driven from a timing heap. Event triggers declare what
they need delivered, which is what lets subscriptions be replayed after a
reconnect rather than silently lost.

`StateChanged` narrows by transition and can require the state to persist:

```go
ha.StateChanged("binary_sensor.motion").To("off").For(5 * time.Minute)
```

Sun times come from Home Assistant's own `sun.sun` entity, not from local
astronomy. Home Assistant runs astral against your latitude, longitude *and*
elevation with a configurable solar depression, so computing them here would
quietly disagree with the times on your own dashboard.

### Conditions

Conditions compose, and an error from one means *undecided* rather than false:

```go
When(ha.All(
	ha.SunIsDown(),
	ha.Not(ha.StateIs("input_boolean.guest_mode", "on")),
	ha.Any(ha.OnWeekdays(time.Saturday, time.Sunday), ha.TimeBetween(ha.TimeOfDay(18, 0), ha.TimeOfDay(23, 0))),
))
```

If a condition cannot be evaluated — an entity is unreachable, say — the
automation's `OnConditionError` setting decides what happens. The default is
`SkipRun`; use `RunAnyway` where not acting is the more dangerous outcome.

### Policy

`Mode` mirrors Home Assistant's automation `mode:` and applies to the
automation as a whole:

| Mode | Behaviour when a run is already in flight |
| --- | --- |
| `ModeSingle` (default) | ignore the new trigger |
| `ModeRestart` | cancel the running one, whose context is then done |
| `ModeQueued` | wait for it, then run |
| `ModeParallel` | run alongside it |

`Throttle` is counted **per entity**, so one automation watching many entities
keeps a separate window for each rather than letting a busy one starve the rest.

### Actions

An action receives a context and a `Run`:

```go
Do(func(ctx context.Context, run ha.Run) error {
	if run.Event.To.State == "on" {
		return run.Services.Light.TurnOn("light.hall")
	}
	return run.Services.Light.TurnOff("light.hall")
})
```

Returning an error logs it; the automation stays live. Under `ModeRestart` the
context is cancelled when a newer trigger arrives, so long-running actions
should respect it.

## Generating entity constants

`cmd/generate` reads your Home Assistant and writes an `entities` package with
a constant per entity, typed by domain.

1. Create `gen.yaml`:

```yaml
url: "http://192.168.1.123:8123"
ha_auth_token: "your_auth_token" # Or set HA_AUTH_TOKEN env var

# Optional: only these domains are processed.
include_domains: ["light", "switch", "climate"]

# Optional: skipped, and only consulted when include_domains is empty.
exclude_domains: ["device_tracker", "person"]
```

2. Add a directive and run it:

```go
//go:generate go run github.com/Xevion/go-ha/cmd/generate
```

```bash
go generate
```

Entities are typed per domain, so a mismatch does not compile:

```go
run.Services.Light.TurnOn(entities.Light.LivingRoom)  // fine
run.Services.Light.TurnOn(entities.Switch.Kitchen)    // build error
```

Constants are named from the entity ID, not from its friendly name.

## Testing your automations

`hatest` runs an in-process Home Assistant, so automations can be tested
without one:

```go
func TestHallLight(t *testing.T) {
	server := hatest.New(t)
	server.SetState("binary_sensor.hall_motion", "off")

	app, err := ha.NewApp(types.NewAppRequest{URL: server.URL(), HAAuthToken: hatest.Token})
	require.NoError(t, err)
	defer app.Close()

	require.NoError(t, app.RegisterAutomations(hallLight()))
	go func() { _ = app.Start() }()

	server.ChangeState("binary_sensor.hall_motion", "on")

	calls := server.WaitForCalls(1)
	assert.Equal(t, "turn_on", calls[0].Service)
}
```

Supply a `Clock` to step time by hand, so a schedule, a throttle window or a
`For` duration resolves on demand rather than in real time:

```go
clock := hatest.NewClock(time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC))
app, err := ha.NewApp(types.NewAppRequest{
	URL:         server.URL(),
	HAAuthToken: hatest.Token,
	Clock:       clock,
})

// ...trigger the automation once, then move past its throttle window.
clock.Advance(time.Hour)
```

## Connection handling

The client owns one websocket connection and re-establishes it with exponential
backoff and jitter when it drops. Subscriptions are declarative and replayed on
every reconnect.

Entity state is cached locally: seeded from the REST API on each connection and
maintained from the event stream, so a condition costs a map lookup rather than
an HTTP round trip, and automations keep working through a disconnect.

Events are read into a bounded queue and handled by a worker pool. Home
Assistant disconnects a client that stops draining its socket for five seconds,
so the queue is deliberately finite: shedding load is survivable, being
disconnected is not. Drops are reported.

Tune it if the defaults do not suit:

```go
ha.NewApp(types.NewAppRequest{
	URL:         "...",
	HAAuthToken: "...",
	Connection: types.ConnectionOptions{
		QueueSize:    512,
		Workers:      8,
		PingInterval: 15 * time.Second,
	},
})
```

## Running it

There is no runtime to install and no container to build. It is an ordinary Go
binary: build it and run it under systemd, a cron job, `tmux`, Docker, or
whatever else you already use.

## Credits

A fork of [saml-dev/gome-assistant](https://github.com/saml-dev/gome-assistant).
