package webhook

import (
	"context"
	"fmt"
	"time"

	"go.miloapis.com/email-provider-resend/internal/resend"
	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/api/meta"

	contactcontroller "go.miloapis.com/email-provider-resend/internal/controller"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create

func NewResendContactWebhookV1(k8sClient client.Client) *Webhook {
	return &Webhook{
		Handler: HandlerFunc(func(ctx context.Context, req Request) Response {
			contactEvent := req.ContactEvent
			log := logf.FromContext(ctx).WithName("resend-webhook")
			log.Info("Received event", "event", contactEvent.Envelope.Type)

			// Getting Contact CR by ProviderID using the indexed field
			contacts := &notificationmiloapiscomv1alpha1.ContactList{}
			if err := k8sClient.List(ctx, contacts, client.MatchingFields{contactStatusProviderIDIndexKey: contactEvent.Contact.ID}); err != nil {
				log.Error(err, "Failed to list contacts by providerID", "providerID", contactEvent.Contact.ID)
				return InternalServerErrorResponse()
			}
			if len(contacts.Items) == 0 {
				log.Info("No contact found with providerID. Probably deleted.", "providerID", contactEvent.Contact.ID)
				return OkResponse()
			}
			contact := &contacts.Items[0]

			updatedCond := meta.FindStatusCondition(contact.Status.Conditions, notificationmiloapiscomv1alpha1.ContactUpdatedCondition)

			// Update contact status
			condition := metav1.Condition{}
			switch contactEvent.Envelope.Type {
			case resend.ContactCreated:
				if updatedCond != nil && updatedCond.Reason == notificationmiloapiscomv1alpha1.ContactUpdatePendingReason {
					// Confirm previously pending update instead of marking deleted.
					condition = metav1.Condition{
						Type:               notificationmiloapiscomv1alpha1.ContactUpdatedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             notificationmiloapiscomv1alpha1.ContactUpdatedReason,
						Message:            "Contact update confirmed by email provider webhook",
						LastTransitionTime: metav1.Now(),
						ObservedGeneration: contact.GetGeneration(),
					}
				} else {
					condition = metav1.Condition{
						Type:               contactcontroller.ResendContactReadyCondition,
						Status:             metav1.ConditionTrue,
						Reason:             contactcontroller.ResendContactCreatedReason,
						Message:            "Contact creation confirmed by email provider webhook",
						LastTransitionTime: metav1.Now(),
						ObservedGeneration: contact.GetGeneration(),
					}
				}
			case resend.ContactUpdated:
				if updatedCond != nil && updatedCond.Reason == notificationmiloapiscomv1alpha1.ContactUpdatePendingReason {
					// Confirm previously pending update instead of marking deleted.
					condition = metav1.Condition{
						Type:               notificationmiloapiscomv1alpha1.ContactUpdatedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             notificationmiloapiscomv1alpha1.ContactUpdatedReason,
						Message:            "Contact update confirmed by email provider webhook",
						LastTransitionTime: metav1.Now(),
						ObservedGeneration: contact.GetGeneration(),
					}
				} else {
					condition = metav1.Condition{
						Type:               notificationmiloapiscomv1alpha1.ContactUpdatedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             notificationmiloapiscomv1alpha1.ContactUpdatedReason,
						Message:            "Contact update confirmed by email provider webhook",
						LastTransitionTime: metav1.Now(),
						ObservedGeneration: contact.GetGeneration(),
					}
				}
			case resend.ContactDeleted:
				if updatedCond != nil && updatedCond.Reason == notificationmiloapiscomv1alpha1.ContactUpdatePendingReason {
					// Confirm previously pending update instead of marking deleted.
					condition = metav1.Condition{
						Type:               notificationmiloapiscomv1alpha1.ContactUpdatedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             notificationmiloapiscomv1alpha1.ContactUpdatedReason,
						Message:            "Contact update confirmed by email provider webhook",
						LastTransitionTime: metav1.Now(),
						ObservedGeneration: contact.GetGeneration(),
					}
				} else {
					condition = metav1.Condition{
						Type:               notificationmiloapiscomv1alpha1.ContactDeletedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             notificationmiloapiscomv1alpha1.ContactDeletedReason,
						Message:            "Contact deletion confirmed by email provider webhook",
						LastTransitionTime: metav1.Now(),
						ObservedGeneration: contact.GetGeneration(),
					}
				}
			}

			meta.SetStatusCondition(&contact.Status.Conditions, condition)
			if err := k8sClient.Status().Update(ctx, contact); err != nil {
				log.Error(err, "Failed to update Contact status", "contactGroupMembership", contact.Name)
				return InternalServerErrorResponse()
			}

			// Emit Kubernetes Event for observability
			webhookEvent := &eventsv1.Event{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: fmt.Sprintf("%s-", contact.Name),
					Namespace:    contact.Namespace,
				},
				Action:              "Update",
				Reason:              condition.Reason,
				Note:                condition.Message,
				Type:                corev1.EventTypeNormal,
				EventTime:           metav1.MicroTime{Time: time.Now()},
				ReportingController: "email-provider-resend-webhook",
				ReportingInstance:   "email-provider-resend-webhook-1",
				Regarding: corev1.ObjectReference{
					Kind:            "Contact",
					Namespace:       contact.Namespace,
					Name:            contact.Name,
					UID:             contact.UID,
					ResourceVersion: contact.ResourceVersion,
					APIVersion:      contact.APIVersion,
				},
			}
			if err := k8sClient.Create(ctx, webhookEvent); err != nil {
				log.Error(err, "Failed to create ContactGroupMembership event", "contact", contact.Name)
				return InternalServerErrorResponse()
			}

			log.Info("Updated Contact status from webhook", "contact", contact.Name, "condition", condition)

			return OkResponse()
		}),
		Endpoint: "/apis/emailnotification.k8s.io/v1/resend/contactgroupmemberships",
	}
}
