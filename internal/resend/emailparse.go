package resend

import (
	"encoding/json"
	"fmt"
	"slices"
)

// IsAllowedEvent returns true if the given event type is supported.
func IsAllowedEvent(et EmailEventType) bool {
	return slices.Contains(AllowedEvents, et)
}

// ParseEvent decodes a Resend webhook payload into a ParsedEvent structure.
// It validates the event type and populates additional fields depending on
// the event.
func ParseEmailEvent(body []byte) (*ParsedEvent, error) {
	var env EventEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event envelope: %w", err)
	}

	if !IsAllowedEvent(env.Type) {
		return nil, fmt.Errorf("unsupported event type: %s", env.Type)
	}

	// Decode the common base block
	var base EmailBase
	if err := json.Unmarshal(env.RawData, &base); err != nil {
		return nil, fmt.Errorf("failed to unmarshal base data: %w", err)
	}

	pe := &ParsedEvent{Envelope: env, Base: base}

	// Decode the event-specific data.
	switch env.Type {
	case EventTypeClicked:
		var c struct {
			Click Click `json:"click"`
		}
		if err := json.Unmarshal(env.RawData, &c); err != nil {
			return nil, fmt.Errorf("failed to unmarshal click data: %w", err)
		}
		pe.Click = &c.Click
	case EventTypeBounced:
		var b struct {
			Bounce Bounce `json:"bounce"`
		}
		if err := json.Unmarshal(env.RawData, &b); err != nil {
			return nil, fmt.Errorf("failed to unmarshal bounce data: %w", err)
		}
		pe.Bounce = &b.Bounce
	case EventTypeEmailFailed:
		var f struct {
			Failed Failed `json:"failed"`
		}
		if err := json.Unmarshal(env.RawData, &f); err != nil {
			return nil, fmt.Errorf("failed to unmarshal failed data: %w", err)
		}
		pe.Failed = &f.Failed
	}

	return pe, nil
}
