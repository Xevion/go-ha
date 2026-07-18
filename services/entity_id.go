package services

// Entity ids are typed per domain so that handing one service another's entity
// does not compile. cmd/generate emits values of these types, which is what
// turns a typo like Light.TurnOn(entities.Switch.Kitchen) into a build error
// rather than a service call Home Assistant quietly ignores.
type (
	// EntityID is an entity in any domain, for services that are not domain
	// specific.
	EntityID string

	AlarmControlPanelID EntityID
	ClimateID           EntityID
	CoverID             EntityID
	InputBooleanID      EntityID
	InputButtonID       EntityID
	InputDatetimeID     EntityID
	InputNumberID       EntityID
	InputTextID         EntityID
	LightID             EntityID
	LockID              EntityID
	MediaPlayerID       EntityID
	NumberID            EntityID
	SceneID             EntityID
	ScriptID            EntityID
	SwitchID            EntityID
	TimerID             EntityID
	VacuumID            EntityID
)

// DomainIDTypes maps a Home Assistant domain to the id type cmd/generate
// should emit for it. Domains absent from it fall back to EntityID.
var DomainIDTypes = map[string]string{
	"alarm_control_panel": "AlarmControlPanelID",
	"climate":             "ClimateID",
	"cover":               "CoverID",
	"input_boolean":       "InputBooleanID",
	"input_button":        "InputButtonID",
	"input_datetime":      "InputDatetimeID",
	"input_number":        "InputNumberID",
	"input_text":          "InputTextID",
	"light":               "LightID",
	"lock":                "LockID",
	"media_player":        "MediaPlayerID",
	"number":              "NumberID",
	"scene":               "SceneID",
	"script":              "ScriptID",
	"switch":              "SwitchID",
	"timer":               "TimerID",
	"vacuum":              "VacuumID",
}
