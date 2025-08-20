package webhook

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"go.miloapis.com/email-provider-resend/internal/resend"
	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"
)

type Webhook struct {
	Handler  Handler
	Endpoint string
}

// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create

func NewResendWebhookV1(k8sClient client.Client) *Webhook {
	return &Webhook{
		Handler: HandlerFunc(func(ctx context.Context, req Request) Response {
			log := logf.FromContext(ctx).WithName("resend-webhook")
			log.Info("Received event", "event", req.Event.Envelope.Type)

			// Getting Email CR by ProviderID using the indexed field
			emails := &notificationmiloapiscomv1alpha1.EmailList{}
			if err := k8sClient.List(ctx, emails, client.MatchingFields{"status.providerID": req.Event.Base.EmailID}); err != nil {
				log.Error(err, "Failed to list emails by providerID", "providerID", req.Event.Base.EmailID)
				return InternalServerErrorResponse()
			}
			if len(emails.Items) == 0 {
				log.Info("No email found with providerID", "providerID", req.Event.Base.EmailID)
				return NotFoundResponse()
			}
			email := &emails.Items[0]

			emailCondition := resend.GetEmailCondition(req.Event.Envelope.Type)

			// Update email status
			condition := metav1.Condition{
				Type:               notificationmiloapiscomv1alpha1.EmailDeliveredCondition,
				Status:             emailCondition.Status,
				Reason:             emailCondition.EmailDeliveredReason,
				Message:            fmt.Sprintf("Updated Email status from webhook event: %s", req.Event.Envelope.Type),
				LastTransitionTime: metav1.Now(),
			}
			meta.SetStatusCondition(&email.Status.Conditions, condition)
			if err := k8sClient.Status().Update(ctx, email); err != nil {
				log.Error(err, "Failed to update Email status", "email", email.Name)
				return InternalServerErrorResponse()
			}

			// Emit Kubernetes Event for observability
			webhookEvent := &eventsv1.Event{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: fmt.Sprintf("%s-", email.Name),
					Namespace:    email.Namespace,
				},
				Action:              "Update",
				Reason:              condition.Reason,
				Note:                condition.Message,
				Type:                emailCondition.CoreEventType,
				EventTime:           metav1.MicroTime{Time: time.Now()},
				ReportingController: "email-provider-resend-webhook",
				ReportingInstance:   "email-provider-resend-webhook-1",
				Regarding: corev1.ObjectReference{
					Kind:            "Email",
					Namespace:       email.Namespace,
					Name:            email.Name,
					UID:             email.UID,
					ResourceVersion: email.ResourceVersion,
					APIVersion:      email.APIVersion,
				},
			}
			if err := k8sClient.Create(ctx, webhookEvent); err != nil {
				log.Error(err, "Failed to create Event", "email", email.Name)
				return InternalServerErrorResponse()
			}

			log.Info("Updated Email status from webhook", "email", email.Name, "condition", condition)

			return OkResponse()
		}),
		Endpoint: "/apis/emailnotification.k8s.io/v1/resend/emails",
	}
}
