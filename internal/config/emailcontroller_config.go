package config

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/validation/field"

	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"
)

type waitBeforeRetry struct {
	low    time.Duration
	normal time.Duration
	high   time.Duration
}

type EmailControllerConfig struct {
	emailPriority waitBeforeRetry
}

// NewEmailControllerConfig creates a new EmailControllerConfig.
func NewEmailControllerConfig(lowPriorityEmailWait, normalPriorityEmailWait, highPriorityEmailWait time.Duration) (*EmailControllerConfig, error) {
	var errs field.ErrorList

	if lowPriorityEmailWait < 0 {
		errs = append(errs, field.Required(field.NewPath("emailPriority.low"), "emailPriority.low must be greater than 0"))
	}
	if normalPriorityEmailWait < 0 {
		errs = append(errs, field.Required(field.NewPath("emailPriority.normal"), "emailPriority.normal must be greater than 0"))
	}
	if highPriorityEmailWait < 0 {
		errs = append(errs, field.Required(field.NewPath("emailPriority.high"), "email.Priority.high must be greater than 0"))
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("invalid email controller config: %w", errs.ToAggregate())
	}

	return &EmailControllerConfig{
		emailPriority: waitBeforeRetry{
			low:    lowPriorityEmailWait,
			normal: normalPriorityEmailWait,
			high:   highPriorityEmailWait,
		},
	}, nil
}

// GetWaitTimeBeforeRetry returns the wait before retry for the given priority.
// If the priority is not found, it returns the normal wait.
func (c *EmailControllerConfig) GetWaitTimeBeforeRetry(priority notificationmiloapiscomv1alpha1.EmailPriority) time.Duration {
	var wait time.Duration
	switch priority {
	case notificationmiloapiscomv1alpha1.EmailPriorityLow:
		wait = c.emailPriority.low
	case notificationmiloapiscomv1alpha1.EmailPriorityNormal:
		wait = c.emailPriority.normal
	case notificationmiloapiscomv1alpha1.EmailPriorityHigh:
		wait = c.emailPriority.high
	default:
		wait = c.emailPriority.normal
	}

	return wait
}
