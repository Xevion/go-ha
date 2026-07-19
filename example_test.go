package ha_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	ha "github.com/Xevion/go-ha"
	"github.com/Xevion/go-ha/hatest"
	"github.com/Xevion/go-ha/types"
)

// Example exercises an automation end to end against an in-process Home
// Assistant, which is what the hatest package is for: no real instance, and the
// automation is asserted on by the service call it made.
func Example() {
	server := hatest.Start()
	defer server.Close()
	server.SetState("binary_sensor.motion", "off")
	server.SetState("light.hall", "off")

	app, err := ha.NewApp(types.NewAppRequest{
		URL:         server.URL(),
		HAAuthToken: hatest.Token,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()

	if err := app.RegisterAutomations(
		ha.NewAutomation("hall light").
			On(ha.StateChanged("binary_sensor.motion").To("on")).
			Do(func(ctx context.Context, run ha.Run) error {
				return run.Services.Light.TurnOn("light.hall")
			}).
			MustBuild(),
	); err != nil {
		log.Fatal(err)
	}

	go app.Start()
	time.Sleep(100 * time.Millisecond) // let the connection and subscription establish

	server.ChangeState("binary_sensor.motion", "on")

	call := server.WaitForCalls(1)[0]
	fmt.Printf("%s.%s %s\n", call.Domain, call.Service, call.EntityID)
	// Output: light.turn_on light.hall
}

// Example_connectingToHomeAssistant is the starting point for a real program:
// connect, register automations, and run until stopped. It talks to a live
// instance, so it is shown rather than run.
func Example_connectingToHomeAssistant() {
	app, err := ha.NewApp(types.NewAppRequest{
		URL:         "http://localhost:8123",
		HAAuthToken: os.Getenv("HA_TOKEN"),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()

	if err := app.RegisterAutomations(
		ha.NewAutomation("porch light").
			On(ha.Sunset()).
			Do(func(ctx context.Context, run ha.Run) error {
				return run.Services.Light.TurnOn("light.porch")
			}).
			MustBuild(),
	); err != nil {
		log.Fatal(err)
	}

	// Start blocks until the context is cancelled or the connection is lost.
	log.Fatal(app.Start())
}

// ExampleNewAutomation builds a rule from a mixed trigger list, a condition, and
// a policy. It fires when motion is seen or at sunset, but only after dark, and
// restarts rather than stacking if it is already running.
func ExampleNewAutomation() {
	automation, err := ha.NewAutomation("hallway light").
		On(ha.StateChanged("binary_sensor.hall").To("on"), ha.Sunset()).
		When(ha.SunIsDown()).
		Mode(ha.ModeRestart).
		Throttle(30 * time.Second).
		Do(func(ctx context.Context, run ha.Run) error {
			return run.Services.Light.TurnOn("light.hallway")
		}).
		Build()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(automation)
	// Output: hallway light (2 trigger(s), restart)
}

// ExampleStateChanged narrows a trigger to one transition, held for a duration:
// the door going from closed to open and staying open for two minutes.
func ExampleStateChanged() {
	automation := ha.NewAutomation("door left open").
		On(ha.StateChanged("binary_sensor.front_door").
			From("off").
			To("on").
			For(2 * time.Minute)).
		Do(func(ctx context.Context, run ha.Run) error {
			return run.Services.Light.TurnOn("light.entry")
		}).
		MustBuild()

	fmt.Println(automation)
	// Output: door left open (1 trigger(s), single)
}

// ExampleAll composes conditions and evaluates the result on its own, against a
// clock under the test's control. Conditions do not need a running app to check.
func ExampleAll() {
	workingHours := ha.All(
		ha.AfterTime(ha.TimeOfDay(9, 0)),
		ha.BeforeTime(ha.TimeOfDay(17, 0)),
	)

	at := func(hour int) ha.EvalContext {
		return ha.EvalContext{Clock: fixedClock{at: time.Date(2026, 7, 19, hour, 0, 0, 0, time.UTC)}}
	}

	noon, _ := workingHours.Eval(context.Background(), at(12))
	evening, _ := workingHours.Eval(context.Background(), at(20))
	fmt.Println(noon, evening)
	// Output: true false
}

// ExampleTimeBetween shows a window that crosses midnight: 22:00 to 06:00 holds
// at 23:30.
func ExampleTimeBetween() {
	overnight := ha.TimeBetween(ha.TimeOfDay(22, 0), ha.TimeOfDay(6, 0))

	ec := ha.EvalContext{Clock: fixedClock{at: time.Date(2026, 7, 19, 23, 30, 0, 0, time.UTC)}}
	held, _ := overnight.Eval(context.Background(), ec)
	fmt.Println(held)
	// Output: true
}

// ExampleApp_State reads an entity's current state, served from the local cache
// the app maintains. It talks to a live instance, so it is shown rather than run.
func ExampleApp_State() {
	app, err := ha.NewApp(types.NewAppRequest{
		URL:         "http://localhost:8123",
		HAAuthToken: os.Getenv("HA_TOKEN"),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()

	state, err := app.State().Get("light.hall")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(state.State)
}
