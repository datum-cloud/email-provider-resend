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
			cgms := &notificationmiloapiscomv1alpha1.ContactGroupMembershipList{}
			if err := k8sClient.List(ctx, cgms, client.MatchingFields{cgmIndexKey: contactEvent.Contact.ID}); err != nil {
				log.Error(err, "Failed to list contacts by providerID", "providerID", contactEvent.Contact.ID)
				return InternalServerErrorResponse()
			}
			if len(cgms.Items) == 0 {
				log.Info("No contact group membership found with providerID", "providerID", contactEvent.Contact.ID)
				return NotFoundResponse()
			}
			contactGroupMembership := &cgms.Items[0]

			// Update contact status
			condition := metav1.Condition{}
			switch contactEvent.Envelope.Type {
			case resend.ContactCreated:
				condition = metav1.Condition{
					Type:               notificationmiloapiscomv1alpha1.ContactGroupMembershipReadyCondition,
					Status:             metav1.ConditionTrue,
					Reason:             notificationmiloapiscomv1alpha1.ContactGroupMembershipCreatedReason,
					Message:            "Contact group membership creation confirmed by email provider webhook",
					LastTransitionTime: metav1.Now(),
				}
			case resend.ContactUpdated:
				condition = metav1.Condition{
					Type:               notificationmiloapiscomv1alpha1.ContactGroupMembershipUpdatedCondition,
					Status:             metav1.ConditionTrue,
					Reason:             notificationmiloapiscomv1alpha1.ContactGroupMembershipUpdatedReason,
					Message:            "Contact group membership update confirmed by email provider webhook",
					LastTransitionTime: metav1.Now(),
				}
			case resend.ContactDeleted:
				condition = metav1.Condition{
					Type:               notificationmiloapiscomv1alpha1.ContactGroupMembershipDeletedCondition,
					Status:             metav1.ConditionTrue,
					Reason:             notificationmiloapiscomv1alpha1.ContactGroupMembershipDeletedReason,
					Message:            "Contact group membership deletion confirmed by email provider webhook",
					LastTransitionTime: metav1.Now(),
				}
			}
			meta.SetStatusCondition(&contactGroupMembership.Status.Conditions, condition)
			if err := k8sClient.Status().Update(ctx, contactGroupMembership); err != nil {
				log.Error(err, "Failed to update ContactGroupMembership status", "contactGroupMembership", contactGroupMembership.Name)
				return InternalServerErrorResponse()
			}

			// Emit Kubernetes Event for observability
			webhookEvent := &eventsv1.Event{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: fmt.Sprintf("%s-", contactGroupMembership.Name),
					Namespace:    contactGroupMembership.Namespace,
				},
				Action:              "Update",
				Reason:              condition.Reason,
				Note:                condition.Message,
				Type:                corev1.EventTypeNormal,
				EventTime:           metav1.MicroTime{Time: time.Now()},
				ReportingController: "email-provider-resend-webhook",
				ReportingInstance:   "email-provider-resend-webhook-1",
				Regarding: corev1.ObjectReference{
					Kind:            "ContactGroupMembership",
					Namespace:       contactGroupMembership.Namespace,
					Name:            contactGroupMembership.Name,
					UID:             contactGroupMembership.UID,
					ResourceVersion: contactGroupMembership.ResourceVersion,
					APIVersion:      contactGroupMembership.APIVersion,
				},
			}
			if err := k8sClient.Create(ctx, webhookEvent); err != nil {
				log.Error(err, "Failed to create ContactGroupMembership event", "contactGroupMembership", contactGroupMembership.Name)
				return InternalServerErrorResponse()
			}

			log.Info("Updated ContactGroupMembership status from webhook", "contactGroupMembership", contactGroupMembership.Name, "condition", condition)

			return OkResponse()
		}),
		Endpoint: "/apis/emailnotification.k8s.io/v1/resend/contactgroupmemberships",
	}
}
