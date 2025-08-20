package resend

import (
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"
)

type EmailCondition struct {
	Status               metav1.ConditionStatus
	EmailDeliveredReason string
	CoreEventType        string
}

// GetEmailCondition returns the email condition for the given event type.
func GetEmailCondition(eventType EmailEventType) EmailCondition {
	var conditionStatus metav1.ConditionStatus
	var emailDeliveredReason string
	var coreEventType string

	conditionStatus = metav1.ConditionUnknown
	emailDeliveredReason = notificationmiloapiscomv1alpha1.EmailDeliveryPendingReason
	if slices.Contains(successfulDeliveredEventsDeliveredEvents, eventType) {
		conditionStatus = metav1.ConditionTrue
		emailDeliveredReason = notificationmiloapiscomv1alpha1.EmailDeliveredReason
	}
	if slices.Contains(failedDeliveredEvents, eventType) {
		conditionStatus = metav1.ConditionFalse
		emailDeliveredReason = notificationmiloapiscomv1alpha1.EmailDeliveryFailedReason
	}

	coreEventType = corev1.EventTypeWarning
	if slices.Contains(normalDeliveredEvents, eventType) {
		coreEventType = corev1.EventTypeNormal
	}

	return EmailCondition{
		Status:               conditionStatus,
		EmailDeliveredReason: emailDeliveredReason,
		CoreEventType:        coreEventType,
	}

}
