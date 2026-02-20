package services

import (
	"github.com/Xevion/go-ha/internal/connect"
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
](conn *connect.Client) *T {
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

// SetID stamps the request with a connection-scoped id. The client calls this
// at send time, because an id is only meaningful on the connection that
// carries it and a request may outlive the one it was built for.
func (r *BaseServiceRequest) SetID(id int64) { r.Id = id }

func NewBaseServiceRequest(entityId string) BaseServiceRequest {
	request := BaseServiceRequest{
		RequestType: "call_service",
	}

	if entityId != "" {
		request.Target.EntityId = entityId
	}

	return request
}
