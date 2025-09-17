package resend

import (
	"encoding/json"
	"fmt"
	"time"
)

// ResendTime is a custom time type that can handle multiple timestamp formats from Resend
type ResendTime struct {
	time.Time
}

// UnmarshalJSON implements json.Unmarshaler to handle multiple timestamp formats
func (rt *ResendTime) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	// List of timestamp formats that Resend uses
	formats := []string{
		time.RFC3339,                       // "2006-01-02T15:04:05Z07:00"
		time.RFC3339Nano,                   // "2006-01-02T15:04:05.999999999Z07:00"
		"2006-01-02T15:04:05.999999Z07:00", // Variant with 6 digit microseconds
		"2006-01-02T15:04:05.999999+00:00", // With +00:00 timezone
		"2006-01-02 15:04:05.999999+00",    // Space instead of T, no colon in timezone
		"2006-01-02 15:04:05.999999+00:00", // Space instead of T, with colon in timezone
	}

	// Try to parse with each format
	for _, format := range formats {
		if t, err := time.Parse(format, str); err == nil {
			rt.Time = t
			return nil
		}
	}

	return fmt.Errorf("unable to parse timestamp %q with any known format", str)
}

// MarshalJSON implements json.Marshaler to output in RFC3339 format
func (rt ResendTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(rt.Format(time.RFC3339))
}

// EventType enumerates Resend webhook event types.
// It corresponds to the `type` field in webhook payloads.
type EmailEventType string

const (
	EventTypeSent             EmailEventType = "email.sent"
	EventTypeDelivered        EmailEventType = "email.delivered"
	EventTypeDeliveredDelayed EmailEventType = "email.delivery_delayed"
	EventTypeComplained       EmailEventType = "email.complained"
	EventTypeBounced          EmailEventType = "email.bounced"
	EventTypeOpened           EmailEventType = "email.opened"
	EventTypeClicked          EmailEventType = "email.clicked"
	EventTypeEmailFailed      EmailEventType = "email.failed"
	EventTypeScheduled        EmailEventType = "email.scheduled"
)

// AllowedEvents lists every event type currently supported by this SDK.
// The slice keeps declaration order for readability.
var allowedEvents = []EmailEventType{
	EventTypeSent,
	EventTypeDelivered,
	EventTypeDeliveredDelayed,
	EventTypeComplained,
	EventTypeBounced,
	EventTypeOpened,
	EventTypeClicked,
	EventTypeEmailFailed,
	EventTypeScheduled,
}

// successfulDeliveredEvents are events that indicate a successful delivery.
var successfulDeliveredEventsDeliveredEvents = []EmailEventType{
	EventTypeDelivered,
	EventTypeOpened,
	EventTypeClicked,
	EventTypeComplained,
}

// failedDeliveredEvents are events that indicate a failed delivery.
var failedDeliveredEvents = []EmailEventType{
	EventTypeBounced,
	EventTypeEmailFailed,
}

// NormalDeliveredEvents are events that indicate a normal delivery.
var normalDeliveredEvents = []EmailEventType{
	EventTypeDelivered,
	EventTypeOpened,
	EventTypeClicked,
	EventTypeScheduled,
	EventTypeSent,
}

// Tag represents a key/value pair attached to events.
type Tag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// EmailBase contains the common fields for all email events.
type EmailBase struct {
	BroadcastID string     `json:"broadcast_id"`
	CreatedAt   ResendTime `json:"created_at"`
	EmailID     string     `json:"email_id"`
	From        string     `json:"from"`
	To          []string   `json:"to"`
	Subject     string     `json:"subject"`
	Tags        []Tag      `json:"tags"`
}

// Click details for email.clicked event.
type Click struct {
	IPAddress string     `json:"ipAddress"`
	Link      string     `json:"link"`
	Timestamp ResendTime `json:"timestamp"`
	UserAgent string     `json:"userAgent"`
}

// Bounce details for email.bounced event.
type Bounce struct {
	Message string `json:"message"`
	SubType string `json:"subType"`
	Type    string `json:"type"`
}

// Failed details for email.failed event.
type Failed struct {
	Reason string `json:"reason"`
}

// EventEnvelope is the top-level structure sent by Resend webhooks.
type EventEnvelope struct {
	Type      EmailEventType  `json:"type"`
	CreatedAt ResendTime      `json:"created_at"`
	RawData   json.RawMessage `json:"data"`
}

// ParsedEvent groups the envelope and decoded payload.
type ParsedEvent struct {
	Envelope EventEnvelope
	Base     EmailBase

	// Optional payloads depending on the event type.
	Click  *Click
	Bounce *Bounce
	Failed *Failed
}
