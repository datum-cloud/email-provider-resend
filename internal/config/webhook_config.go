package config

import (
	"fmt"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

// WebhookConfig contains the configuration required to start the standalone
// webhook server.
type WebhookConfig struct {
	Port               int
	CertDir            string
	CertFile           string
	KeyFile            string
	MetricsBindAddress string
}

// NewWebhookConfig validates the provided parameters and, if they are correct,
// returns a WebhookConfig instance. If any of the parameters are invalid an
// aggregated error is returned describing all the problems found.
func NewWebhookConfig(port int, certDir, certFile, keyFile, metricsBindAddress string) (*WebhookConfig, error) {
	log := logf.Log.WithName("resendn-webhook")
	var errs field.ErrorList

	if port <= 0 || port > 65535 {
		errs = append(errs, field.Invalid(field.NewPath("port"), port, "must be between 1 and 65535"))
	}

	if certDir == "" {
		errs = append(errs, field.Required(field.NewPath("certDir"), "is required"))
	}

	if certFile == "" {
		errs = append(errs, field.Required(field.NewPath("certFile"), "is required"))
	}

	if keyFile == "" {
		errs = append(errs, field.Required(field.NewPath("keyFile"), "is required"))
	}

	// Validate metrics bind address, it must be in the form host:port (the host
	// can be empty, meaning all interfaces).
	if metricsBindAddress == "" {
		errs = append(errs, field.Required(field.NewPath("metricsBindAddress"), "is required"))
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("invalid webhook config: %w", errs.ToAggregate())
	}

	log.Info("Starting resend webhook server",
		"cert_dir", certDir,
		"cert_file", certFile,
		"key_file", keyFile,
		"webhook_port", port,
	)

	log.Info("Metrics bind address",
		"metrics-bind-address", metricsBindAddress,
	)

	return &WebhookConfig{
		Port:               port,
		CertDir:            certDir,
		CertFile:           certFile,
		KeyFile:            keyFile,
		MetricsBindAddress: metricsBindAddress,
	}, nil
}
