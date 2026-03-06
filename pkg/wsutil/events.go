package wsutil

import "time"

type Event struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

func NewEvent(eventType string, payload interface{}) Event {
	return Event{Type: eventType, Timestamp: time.Now().UTC(), Payload: payload}
}
