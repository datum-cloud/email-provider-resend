/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	iammiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/iam/v1alpha1"
	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"go.miloapis.com/email-provider-resend/internal/config"
	"go.miloapis.com/email-provider-resend/internal/emailprovider"
)

// EmailReconciler reconciles a Email object
type EmailController struct {
	Client        client.Client
	EmailProvider emailprovider.Service
	Config        config.EmailControllerConfig
}

// +kubebuilder:rbac:groups=notification.miloapis.com,resources=emails,verbs=get
// +kubebuilder:rbac:groups=notification.miloapis.com,resources=emails/status,verbs=get
// +kubebuilder:rbac:groups=notification.miloapis.com,resources=emailtemplates,verbs=get
// +kubebuilder:rbac:groups=iam.miloapis.com,resources=users,verbs=get

// Reconcile is the main function that reconciles the Email object.
func (r *EmailController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithName("email-reconciler")

	// Get Email
	email := &notificationmiloapiscomv1alpha1.Email{}
	err := r.Client.Get(ctx, req.NamespacedName, email)
	if errors.IsNotFound(err) {
		log.Info("Email not found. Probably deleted.", "email", req.Name)
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Email", "email", req.Name)
		return ctrl.Result{}, fmt.Errorf("failed to get Email: %w", err)
	}

	log.Info("Reconciling Email", "email", email.Name, "template", email.Spec.TemplateRef.Name, "user", email.Spec.UserRef.Name)

	// Get EmailTemplate
	emailTemplate := &notificationmiloapiscomv1alpha1.EmailTemplate{} // Cluster scoped resource
	err = r.Client.Get(ctx, client.ObjectKey{Name: email.Spec.TemplateRef.Name}, emailTemplate)
	if err != nil {
		// emailTemplate is warranty to exist. As it is checked on a webhook on Milo.
		log.Error(err, "Failed to get EmailTemplate", "email", email.Spec.TemplateRef.Name)
		return ctrl.Result{}, fmt.Errorf("failed to get EmailTemplate: %w", err)
	}

	// Get EmailRecipient
	emailRecipient := &iammiloapiscomv1alpha1.User{} // Cluster scoped resource
	if err = r.Client.Get(ctx, client.ObjectKey{Name: email.Spec.UserRef.Name}, emailRecipient); err != nil {
		// emailRecipient is warranty to exist. As it is checked on a webhook on Milo.
		log.Error(err, "Failed to get EmailRecipient", "email", email.Spec.UserRef.Name)
		return ctrl.Result{}, fmt.Errorf("failed to get EmailRecipient: %w", err)
	}

	if !isEmailAlreadySent(email) {
		log.Info("Sending email")

		// Send email
		output, err := r.EmailProvider.Send(ctx, email.DeepCopy(), emailTemplate.DeepCopy(), emailRecipient.DeepCopy())
		if err != nil {
			log.Error(err, "Failed to send email", "email", email.Name)
			return ctrl.Result{RequeueAfter: r.Config.GetWaitTimeBeforeRetry(email.Spec.Priority)}, nil
		}
		log.Info("Email sent", "email", email.Name, "deliveryID", output.DeliveryID)

		// Record provider ID and mark delivery as pending (Status=Unknown).
		// We set this ONLY on the first successful send so that future webhook
		// updates (Delivered / Failed) are not overwritten by subsequent
		// reconciliations.
		email.Status.ProviderID = output.DeliveryID

		// EmailProvider.Send (resend implementation) uses an idempotency mechanism using the Email.Name as idempotency key.
		// In case of a failure updating the status, the email won't be sent again, and the return value from EmailProvider.Send
		// will be the same one as the original one. The idempotency only lasts for 24 hours.
		if err := r.updateEmailStatus(ctx, email, metav1.Condition{
			Type:               notificationmiloapiscomv1alpha1.EmailDeliveredCondition,
			Status:             metav1.ConditionUnknown,
			Reason:             notificationmiloapiscomv1alpha1.EmailDeliveryPendingReason,
			Message:            fmt.Sprintf("Email accepted for delivery. Provider ID: %s", output.DeliveryID),
			LastTransitionTime: metav1.Now(),
		}); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update Email status: %w", err)
		}

	} else {
		log.Info("Email was already sent. Probably reconciling because of webhook update.")
	}

	log.Info("Email reconciled")
	return ctrl.Result{}, nil
}

// isEmailAlreadyDelivered checks if the email has already been successfully sent
func isEmailAlreadySent(email *notificationmiloapiscomv1alpha1.Email) bool {
	// If we already have a ProviderID, we know the email was at leastaccepted by email provider.
	if email.Status.ProviderID != "" {
		return true
	}

	// If the Delivered condition is already True we also skip.
	if meta.IsStatusConditionTrue(email.Status.Conditions, notificationmiloapiscomv1alpha1.EmailDeliveredCondition) {
		return true
	}

	return false
}

// updateEmailStatus updates the status of the email with the given condition.
func (r *EmailController) updateEmailStatus(ctx context.Context, email *notificationmiloapiscomv1alpha1.Email, condition metav1.Condition) error {
	log := logf.FromContext(ctx).WithName("email-reconciler")

	meta.SetStatusCondition(&email.Status.Conditions, condition)

	if err := r.Client.Status().Update(ctx, email); err != nil {
		log.Error(err, "failed to update Email status", "email", email.Name)
		return fmt.Errorf("failed to update Email status: %w", err)
	}
	log.Info("Email status updated")

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EmailController) SetupWithManager(mgr ctrl.Manager) error {
	// Index Email objects by .status.providerID
	if err := mgr.GetFieldIndexer().IndexField(context.Background(),
		&notificationmiloapiscomv1alpha1.Email{}, "status.providerID",
		func(obj client.Object) []string {
			e := obj.(*notificationmiloapiscomv1alpha1.Email)
			if e.Status.ProviderID == "" {
				return nil
			}
			return []string{e.Status.ProviderID}
		},
	); err != nil {
		return fmt.Errorf("failed to index Email objects by .status.providerID: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&notificationmiloapiscomv1alpha1.Email{}).
		Named("email").
		Complete(r)
}
