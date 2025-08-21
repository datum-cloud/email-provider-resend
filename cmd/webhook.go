package main

import (
	"fmt"

	"github.com/spf13/cobra"
	svixSdk "github.com/svix/svix-webhooks/go"

	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	config "go.miloapis.com/email-provider-resend/internal/config"
	webhook "go.miloapis.com/email-provider-resend/internal/webhook"
	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"
)

// createWebhookCommand returns a cobra command that starts the Resend webhook server.
func createWebhookCommand() *cobra.Command {
	var webhookPort int
	var certDir, certFile, keyFile string
	var metricsBindAddress string
	var webhookSigningKey string

	cmd := &cobra.Command{
		Use:   "resend-webhook",
		Short: "Runs the Resend webhook server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWebhook(
				cmd,
				webhookPort,
				certDir, certFile, keyFile,
				metricsBindAddress,
				webhookSigningKey)
		},
	}

	// Network & Kubernetes flags.
	cmd.Flags().IntVar(&webhookPort, "webhook-port", 9443, "Port for the webhook server")
	cmd.Flags().StringVar(&certDir, "cert-dir", "/etc/certs",
		"Directory that contains the TLS certs to use for serving the webhook")
	cmd.Flags().StringVar(&certFile, "cert-file", "", "Filename in the directory that contains the TLS cert")
	cmd.Flags().StringVar(&keyFile, "key-file", "", "Filename in the directory that contains the TLS private key")

	// Webhook flags
	cmd.Flags().StringVar(&webhookSigningKey, "resend-webhook-signing-key", "",
		"*Required. Signing key for the resendwebhook. This is used to verify the webhook requests from Resend.")

	// Metrics flags.
	cmd.Flags().StringVar(&metricsBindAddress, "metrics-bind-address", ":8080", "address the metrics endpoint binds to")

	return cmd
}

func runWebhook(
	cmd *cobra.Command,
	webhookPort int,
	certDir, certFile, keyFile string,
	metricsBindAddress string,
	webhookSigningKey string) error {
	logf.SetLogger(zap.New(zap.JSONEncoder()))
	log := logf.Log.WithName("resend-webhook")

	// Validate command flags early so we fail fast with meaningful errors.
	if _, err := config.NewWebhookConfig(
		webhookPort, certDir, certFile, keyFile,
		metricsBindAddress,
		webhookSigningKey); err != nil {
		return err
	}

	// Setup Kubernetes client config
	restConfig, err := k8sconfig.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get rest config: %w", err)
	}

	runtimeScheme := runtime.NewScheme()
	if err := notificationmiloapiscomv1alpha1.AddToScheme(runtimeScheme); err != nil {
		return fmt.Errorf("failed to add notificationmiloapiscomv1alpha1 scheme: %w", err)
	}
	if err := eventsv1.AddToScheme(runtimeScheme); err != nil {
		return fmt.Errorf("failed to add eventsv1 scheme: %w", err)
	}

	log.Info("Creating manager")
	mgr, err := manager.New(restConfig, manager.Options{
		Scheme: runtimeScheme,
		Metrics: server.Options{
			BindAddress: metricsBindAddress,
		},
		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			CertDir:  certDir,
			CertName: certFile,
			KeyName:  keyFile,
			Port:     webhookPort,
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	log.Info("Setting up webhook server")

	webhookv1 := webhook.NewResendWebhookV1(mgr.GetClient())
	err = webhookv1.SetupWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to setup webhook: %w", err)
	}

	log.Info("Setting up svix for webhook")
	svix, err := svixSdk.NewWebhook(webhookSigningKey)
	if err != nil {
		return fmt.Errorf("failed to create svix webhook: %w", err)
	}
	webhookv1.SetupSvix(svix)

	log.Info("Starting manager")
	return mgr.Start(cmd.Context())
}
