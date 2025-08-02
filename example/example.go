package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"time"

	// "example/entities" // Optional import generated entities
	ha "github.com/Xevion/go-ha"
)

//go:generate go run github.com/Xevion/go-ha/cmd/generate

func main() {
	app, err := ha.NewApp(ha.NewAppRequest{
		URL:              "http://192.168.86.67:8123", // Replace with your Home Assistant URL
		HAAuthToken:      os.Getenv("HA_AUTH_TOKEN"),
		HomeZoneEntityId: "zone.home",
	})
	if err != nil {
		slog.Error("Error connecting to HASS:", "error", err)
		os.Exit(1)
	}

	defer func() {
		slog.Info("Shutting down application...")
		if err := app.Close(); err != nil {
			slog.Error("Error during shutdown", "error", err)
		}
		slog.Info("Application shutdown complete")
	}()

	pantryDoor := ha.
		NewEntityListener().
		EntityIds(entities.BinarySensor.PantryDoor). // Use generated entity constant
		Call(pantryLights).
		Build()

	_11pmSched := ha.
		NewDailySchedule().
		Call(lightsOut).
		At("23:00").
		Build()

	_30minsBeforeSunrise := ha.
		NewDailySchedule().
		Call(sunriseSched).
		Sunrise("-30m").
		Build()

	zwaveEventListener := ha.
		NewEventListener().
		EventTypes("zwave_js_value_notification").
		Call(onEvent).
		Build()

	app.RegisterEntityListeners(pantryDoor)
	app.RegisterSchedules(_11pmSched, _30minsBeforeSunrise)
	app.RegisterEventListeners(zwaveEventListener)

	app.Start()
}

func pantryLights(service *ha.Service, state ha.State, sensor ha.EntityData) {
	l := "light.pantry"
	// l := entities.Light.Pantry // Or use generated entity constant
	if sensor.ToState == "on" {
		service.HomeAssistant.TurnOn(l)
	} else {
		service.HomeAssistant.TurnOff(l)
	}
}

func onEvent(service *ha.Service, state ha.State, data ha.EventData) {
	// Since the structure of the event changes depending
	// on the event type, you can Unmarshal the raw json
	// into a Go type. If a type for your event doesn't
	// exist, you can write it yourself! PR's welcome to
	// the eventTypes.go file :)
	ev := ha.EventZWaveJSValueNotification{}
	json.Unmarshal(data.RawEventJSON, &ev)
	slog.Info("On event invoked", "event", ev)
}

func lightsOut(service *ha.Service, state ha.State) {
	// always turn off outside lights
	service.Light.TurnOff(entities.Light.OutsideLights)
	s, err := state.Get(entities.BinarySensor.LivingRoomMotion)
	if err != nil {
		slog.Warn("couldnt get living room motion state, doing nothing")
		return
	}

	// if no motion detected in living room for 30mins
	if s.State == "off" && time.Since(s.LastChanged).Minutes() > 30 {
		service.Light.TurnOff(entities.Light.MainLights)
	}
}

func sunriseSched(service *ha.Service, state ha.State) {
	service.Light.TurnOn(entities.Light.LivingRoomLamps)
	service.Light.TurnOff(entities.Light.ChristmasLights)
}
