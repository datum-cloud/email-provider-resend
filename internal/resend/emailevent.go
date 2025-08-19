package resend

import (
	"encoding/json"
	"time"
)

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
var AllowedEvents = []EmailEventType{
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

// Tag represents a key/value pair attached to events.
type Tag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// EmailBase contains the common fields for all email events.
type EmailBase struct {
	BroadcastID string    `json:"broadcast_id"`
	CreatedAt   time.Time `json:"created_at"`
	EmailID     string    `json:"email_id"`
	From        string    `json:"from"`
	To          []string  `json:"to"`
	Subject     string    `json:"subject"`
	Tags        []Tag     `json:"tags"`
}

// Click details for email.clicked event.
type Click struct {
	IPAddress string    `json:"ipAddress"`
	Link      string    `json:"link"`
	Timestamp time.Time `json:"timestamp"`
	UserAgent string    `json:"userAgent"`
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
	CreatedAt time.Time       `json:"created_at"`
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
