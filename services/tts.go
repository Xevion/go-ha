package services

type TTS struct {
	conn Sender
}

// Remove all text-to-speech cache files and RAM cache.
func (tts TTS) ClearCache() error {
	req := NewBaseServiceRequest("")
	req.Domain = "tts"
	req.Service = "clear_cache"

	return tts.conn.Send(&req)
}

// Say something using text-to-speech on a media player with cloud. Takes an entityId and an optional map that is translated into service_data.
func (tts TTS) CloudSay(entityId EntityID, serviceData ...map[string]any) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "tts"
	req.Service = "cloud_say"
	if len(serviceData) != 0 {
		req.ServiceData = serviceData[0]
	}

	return tts.conn.Send(&req)
}

// Say something using text-to-speech on a media player with google_translate. Takes an entityId and an optional map that is translated into service_data.
func (tts TTS) GoogleTranslateSay(entityId EntityID, serviceData ...map[string]any) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "tts"
	req.Service = "google_translate_say"
	if len(serviceData) != 0 {
		req.ServiceData = serviceData[0]
	}

	return tts.conn.Send(&req)
}
