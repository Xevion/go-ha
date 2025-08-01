package services

import (
	"github.com/Xevion/gome-assistant/internal"
	ws "github.com/Xevion/gome-assistant/internal/websocket"
)

func BuildService[
	T AdaptiveLighting |
		AlarmControlPanel |
		Climate |
		Cover |
		Light |
		HomeAssistant |
		Lock |
		MediaPlayer |
		Switch |
		InputBoolean |
		InputButton |
		InputDatetime |
		InputText |
		InputNumber |
		Event |
		Notify |
		Number |
		Scene |
		Script |
		TTS |
		Timer |
		Vacuum |
		ZWaveJS,
](conn *ws.WebsocketWriter) *T {
	return &T{conn: conn}
}

type BaseServiceRequest struct {
	Id          int64          `json:"id"`
	RequestType string         `json:"type"` // hardcoded "call_service"
	Domain      string         `json:"domain"`
	Service     string         `json:"service"`
	ServiceData map[string]any `json:"service_data,omitempty"`
	Target      struct {
		EntityId string `json:"entity_id,omitempty"`
	} `json:"target,omitempty"`
}

func NewBaseServiceRequest(entityId string) BaseServiceRequest {
	id := internal.NextId()
	request := BaseServiceRequest{
		Id:          id,
		RequestType: "call_service",
	}

	if entityId != "" {
		request.Target.EntityId = entityId
	}

	return request
}
