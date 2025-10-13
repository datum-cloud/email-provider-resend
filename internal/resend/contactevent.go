package resend

import (
	"encoding/json"
	"slices"
)

// ContactEventType enumerates Resend webhook event types related to contacts.
// It corresponds to the `type` field in contact webhook payloads.
type ContactEventType string

const (
	ContactCreated ContactEventType = "contact.created"
	ContactUpdated ContactEventType = "contact.updated"
	ContactDeleted ContactEventType = "contact.deleted"
)

// allowedContactEvents lists every contact event type currently supported.
var allowedContactEvents = []ContactEventType{
	ContactCreated,
	ContactUpdated,
	ContactDeleted,
}

// IsAllowedContactEvent returns true if the given event type is supported.
func IsAllowedContactEvent(et ContactEventType) bool {
	return slices.Contains(allowedContactEvents, et)
}

// ContactBase contains the common fields for all contact events.
type ContactBase struct {
	AudienceID   string     `json:"audience_id"`
	CreatedAt    ResendTime `json:"created_at"`
	Email        string     `json:"email"`
	FirstName    string     `json:"first_name"`
	ID           string     `json:"id"`
	LastName     string     `json:"last_name"`
	Unsubscribed bool       `json:"unsubscribed"`
	UpdatedAt    ResendTime `json:"updated_at"`
}

// ContactEventEnvelope is the top-level structure sent by Resend contact webhooks.
type ContactEventEnvelope struct {
	Type      ContactEventType `json:"type"`
	CreatedAt ResendTime       `json:"created_at"`
	RawData   json.RawMessage  `json:"data"`
}

// ParsedContactEvent groups the envelope and decoded payload.
type ParsedContactEvent struct {
	Envelope ContactEventEnvelope
	Contact  ContactBase
}
