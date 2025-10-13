package resend

import (
	"encoding/json"
	"fmt"
)

// ParseContactEvent decodes a Resend contact webhook payload into a ParsedContactEvent structure.
// It validates the event type and populates the base fields.
func ParseContactEvent(body []byte) (*ParsedContactEvent, error) {
	var env ContactEventEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("failed to unmarshal contact event envelope: %w", err)
	}

	if !IsAllowedContactEvent(env.Type) {
		return nil, fmt.Errorf("unsupported contact event type: %s", env.Type)
	}

	// Decode the base data block.
	var base ContactBase
	if err := json.Unmarshal(env.RawData, &base); err != nil {
		return nil, fmt.Errorf("failed to unmarshal contact base data: %w", err)
	}

	return &ParsedContactEvent{
		Envelope: env,
		Contact:  base,
	}, nil
}
