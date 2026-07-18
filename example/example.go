// Command example is a small go-ha application.
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
		URL:         "http://localhost:8123",
		HAAuthToken: os.Getenv("HA_AUTH_TOKEN"),
	})
	if err != nil {
		log.Fatalf("connecting to Home Assistant: %v", err)
	}
	defer app.Close()

	if err := app.RegisterAutomations(
		hallLight(),
		eveningScene(),
		lightsOutWhenEveryoneLeaves(),
		reportDoorbell(),
	); err != nil {
		log.Fatalf("registering automations: %v", err)
	}

	if err := app.Start(); err != nil {
		log.Fatalf("stopped: %v", err)
	}
}

// hallLight turns the hall light on when motion is seen after dark, and off
// again once the hall has been quiet for five minutes.
func hallLight() ha.Automation {
	motion := ha.StateChanged("binary_sensor.hall_motion")

	return ha.NewAutomation("hall light").
		On(motion.To("on")).
		When(ha.SunIsDown()).
		Throttle(30 * time.Second).
		Do(func(ctx context.Context, run ha.Run) error {
			return run.Services.Light.TurnOn("light.hall")
		}).
		MustBuild()
}

// eveningScene runs at sunset, a quarter of an hour early, on weekdays only.
func eveningScene() ha.Automation {
	return ha.NewAutomation("evening scene").
		On(ha.Sunset(-15 * time.Minute)).
		When(ha.Not(ha.OnWeekdays(time.Saturday, time.Sunday))).
		Do(func(ctx context.Context, run ha.Run) error {
			return run.Services.Scene.TurnOn("scene.evening")
		}).
		MustBuild()
}

// lightsOutWhenEveryoneLeaves waits for the house to stay empty rather than
// reacting to the first door closing, and restarts that wait if anyone returns.
func lightsOutWhenEveryoneLeaves() ha.Automation {
	return ha.NewAutomation("lights out").
		On(ha.StateChanged("group.family").To("not_home").For(5 * time.Minute)).
		Mode(ha.ModeRestart).
		Do(func(ctx context.Context, run ha.Run) error {
			return run.Services.Light.TurnOff("light.hall")
		}).
		MustBuild()
}

// reportDoorbell shows a raw event trigger, for integrations this package does
// not model directly.
func reportDoorbell() ha.Automation {
	return ha.NewAutomation("doorbell").
		On(ha.EventFired("zha_event")).
		Mode(ha.ModeQueued).
		Do(func(ctx context.Context, run ha.Run) error {
			log.Printf("doorbell event: %s", run.Event.Raw)
			return nil
		}).
		MustBuild()
}
