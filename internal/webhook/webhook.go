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

func NewResendWebhook(k8sClient client.Client) *Webhook {
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

			conditionStatus, emailDeliveredReason, eventType, err := getEmailCondition(req.Event.Envelope.Type)
			if err != nil {
				log.Error(err, "Failed to get email condition", "event", req.Event.Envelope.Type)
				return BadRequestResponse()
			}

			// Update email status
			condition := metav1.Condition{
				Type:               notificationmiloapiscomv1alpha1.EmailDeliveredCondition,
				Status:             conditionStatus,
				Reason:             emailDeliveredReason, // default reason
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
				Reason:              condition.Reason,
				Note:                condition.Message,
				Type:                eventType,
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
				log.Error(err, "Failed to create Event", "email", email.Name, "event", webhookEvent)
				return InternalServerErrorResponse()
			}

			log.Info("Updated Email status from webhook", "email", email.Name, "condition", condition)

			return OkResponse()
		}),
		Endpoint: "/apis/emailnotification.k8s.io/v1/resend",
	}
}

func getEmailCondition(resendEventType resend.EmailEventType) (metav1.ConditionStatus, string, string, error) {
	conditionStatus := metav1.ConditionUnknown
	var emailDeliveredReason string
	var eventType string

	// Explanation of each resend event type:
	// https://resend.com/docs/dashboard/webhooks/event-types#email-complained
	switch resendEventType {
	// Email delivered successfully, Normal event
	case resend.EventTypeDelivered, resend.EventTypeOpened, resend.EventTypeClicked:
		emailDeliveredReason = notificationmiloapiscomv1alpha1.EmailDeliveredReason
		eventType = corev1.EventTypeNormal
	// Email delivered successfully, Warning event (Went to spam)
	case resend.EventTypeComplained:
		emailDeliveredReason = notificationmiloapiscomv1alpha1.EmailDeliveredReason
		eventType = corev1.EventTypeWarning
	// Email delivery pending, Normal event
	case resend.EventTypeScheduled, resend.EventTypeSent:
		emailDeliveredReason = notificationmiloapiscomv1alpha1.EmailDeliveryPendingReason
		eventType = corev1.EventTypeNormal
	// Email delivery pending, Warning event
	case resend.EventTypeDeliveredDelayed:
		emailDeliveredReason = notificationmiloapiscomv1alpha1.EmailDeliveryPendingReason
		eventType = corev1.EventTypeWarning
	// Email delivery failed, Warning event
	case resend.EventTypeBounced, resend.EventTypeEmailFailed:
		emailDeliveredReason = notificationmiloapiscomv1alpha1.EmailDeliveryFailedReason
		eventType = corev1.EventTypeWarning
	default:
		return "", "", "", fmt.Errorf("unknown event type: %s", resendEventType)
	}

	if emailDeliveredReason == notificationmiloapiscomv1alpha1.EmailDeliveredReason {
		conditionStatus = metav1.ConditionTrue
	} else {
		conditionStatus = metav1.ConditionFalse
	}

	return conditionStatus, emailDeliveredReason, eventType, nil
}
