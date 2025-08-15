package config

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

type emailProviderConfig struct {
	apiKey  string
	from    string
	replyTo string
}

func NewEmailProviderConfig(apiKey, from, replyTo string) (*emailProviderConfig, error) {
	var errs field.ErrorList

	if apiKey == "" {
		errs = append(errs, field.Required(field.NewPath("apiKey"), "apiKey is required"))
	}
	if from == "" {
		errs = append(errs, field.Required(field.NewPath("from"), "from is required"))
	}
	if replyTo == "" {
		errs = append(errs, field.Required(field.NewPath("replyTo"), "replyTo is required"))
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("invalid email provider config: %w", errs.ToAggregate())
	}

	return &emailProviderConfig{
		apiKey:  apiKey,
		from:    from,
		replyTo: replyTo,
	}, nil
}

func (c *emailProviderConfig) GetAPIKey() string {
	return c.apiKey
}

func (c *emailProviderConfig) GetFrom() string {
	return c.from
}

func (c *emailProviderConfig) GetReplyTo() string {
	return c.replyTo
}
