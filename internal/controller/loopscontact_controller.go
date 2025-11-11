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

	"go.miloapis.com/email-provider-resend/internal/emailprovider"
	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/finalizer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	loopsContactFinalizerKey = "notification.miloapis.com/loops-contact"
)

const (
	// LoopsContactReadyCondition is a condition that is set to true when the Loops contact is ready
	LoopsContactReadyCondition = "LoopsContactReady"
	// ContactNotCreatedReason is a reason that is set when the Loops contact is not created
	LoopsContactNotCreatedReason = "ContactNotCreated"
	// ContactCreatedReason is a reason that is set when the Loops contact is created
	LoopsContactCreatedReason = "ContactCreated"
	// ContactUpdatedReason is a reason that is set when the Loops contact is updated
	LoopsContactUpdatedReason = "ContactUpdated"
	// ContactNotUpdatedReason is a reason that is set when the Loops contact is not updated
	LoopsContactNotUpdatedReason = "ContactNotUpdated"
)

// LoopsContactReconciler reconciles a LoopsContact object
type LoopsContactController struct {
	Client     client.Client
	Finalizers finalizer.Finalizers
	Loops      emailprovider.LoopsEmail
}

// loopsContactFinalizer is a finalizer for the Contact object
type loopsContactFinalizer struct {
	Client client.Client
	Loops  emailprovider.LoopsEmail
}

func (f *loopsContactFinalizer) Finalize(ctx context.Context, obj client.Object) (finalizer.Result, error) {
	log := logf.FromContext(ctx).WithValues("finalizer", "ContactFinalizer", "trigger", obj.GetName())
	log.Info("Finalizing Contact")

	// Type assertion
	contact, ok := obj.(*notificationmiloapiscomv1alpha1.Contact)
	if !ok {
		log.Error(fmt.Errorf("object is not a Contact"), "Failed to finalize Contact")
		return finalizer.Result{}, fmt.Errorf("object is not a Contact")
	}

	// Delete Loops contact
	err := f.DeleteContact(ctx, contact)
	if err != nil {
		log.Error(err, "Failed to delete Loops contact")
		return finalizer.Result{}, fmt.Errorf("failed to delete Loops contact: %w", err)
	}

	return finalizer.Result{}, nil
}

// +kubebuilder:rbac:groups=notification.miloapis.com,resources=contacts,verbs=get;list;watch
// +kubebuilder:rbac:groups=notification.miloapis.com,resources=contacts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=notification.miloapis.com,resources=contacts/finalizers,verbs=update
// +kubebuilder:rbac:groups=notification.miloapis.com,resources=contactgroupmemberships,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=email.loops.com,resources=contacts,verbs=get;list;watch;delete

// Reconcile is the main function that reconciles the Contact object.
func (r *LoopsContactController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("controller", "ContactController", "trigger", req.NamespacedName)
	log.Info("Starting reconciliation", "namespacedName", req.String(), "name", req.Name, "namespace", req.Namespace)

	// Get Contact
	contact := &notificationmiloapiscomv1alpha1.Contact{}
	err := r.Client.Get(ctx, req.NamespacedName, contact)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Contact not found. Probably deleted.")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get contact: %w", err)
	}

	// Run finalizers
	finalizeResult, err := r.Finalizers.Finalize(ctx, contact)
	if err != nil {
		log.Error(err, "Failed to run finalizers for Contact")
		return ctrl.Result{}, fmt.Errorf("failed to run finalizers for Contact: %w", err)
	}
	if finalizeResult.Updated {
		log.Info("finalizer updated the contact object, updating API server")
		if updateErr := r.Client.Update(ctx, contact); updateErr != nil {
			log.Error(updateErr, "Failed to update Contact after finalizer update")
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, nil
	}

	oldStatus := contact.Status.DeepCopy()
	readyCond := meta.FindStatusCondition(contact.Status.Conditions, LoopsContactReadyCondition)

	switch {
	// First creation – condition not present yet
	case readyCond == nil:
		log.Info("LoopsContact creation")

		created, err := r.createContact(ctx, contact)
		if err != nil && !errors.IsConflict(err) {
			log.Error(err, "Failed to create Loops contact")
			return ctrl.Result{}, fmt.Errorf("failed to create Loops contact: %w", err)
		}

		if err != nil && errors.IsConflict(err) {
			log.Info("Loops contact already exists")
			meta.SetStatusCondition(&contact.Status.Conditions, metav1.Condition{
				Type:               LoopsContactReadyCondition,
				Status:             metav1.ConditionFalse,
				Reason:             LoopsContactNotCreatedReason,
				Message:            fmt.Sprintf("Loops contact not created on email provider: %s", err.Error()),
				LastTransitionTime: metav1.Now(),
				ObservedGeneration: contact.GetGeneration(),
			})
		}

		if err == nil {
			log.Info("Loops contact created")
			meta.SetStatusCondition(&contact.Status.Conditions, metav1.Condition{
				Type:               LoopsContactReadyCondition,
				Status:             metav1.ConditionTrue,
				Reason:             LoopsContactCreatedReason,
				Message:            "Loops contact created on email provider",
				LastTransitionTime: metav1.Now(),
				ObservedGeneration: contact.GetGeneration(),
			})
			contact.Status.Providers = []notificationmiloapiscomv1alpha1.ContactProviderStatus{
				{
					Name: "Loops",
					ID:   created.ID,
				},
			}
		}

	// Update – generation changed since we last processed the object
	case readyCond.ObservedGeneration != contact.GetGeneration():
		log.Info("Contact updated")

		err := r.updateContact(ctx, contact)
		if err != nil && !errors.IsConflict(err) {
			log.Error(err, "Failed to update Loops contact")
			return ctrl.Result{}, fmt.Errorf("failed to update Loops contact: %w", err)
		}

		if err != nil && errors.IsConflict(err) {
			log.Info("Loops contact already exists")
			meta.SetStatusCondition(&contact.Status.Conditions, metav1.Condition{
				Type:               LoopsContactReadyCondition,
				Status:             metav1.ConditionFalse,
				Reason:             LoopsContactNotUpdatedReason,
				Message:            fmt.Sprintf("Loops contact not updated on email provider: %s", err.Error()),
				LastTransitionTime: metav1.Now(),
				ObservedGeneration: contact.GetGeneration(),
			})
		}

		if err == nil {
			log.Info("Loops contact updated")
			meta.SetStatusCondition(&contact.Status.Conditions, metav1.Condition{
				Type:               LoopsContactReadyCondition,
				Status:             metav1.ConditionTrue,
				Reason:             LoopsContactUpdatedReason,
				Message:            "Loops contact updated on email provider",
				LastTransitionTime: metav1.Now(),
				ObservedGeneration: contact.GetGeneration(),
			})
		}

	}

	// Update contact status if it changed
	original := contact.DeepCopy()
	if !equality.Semantic.DeepEqual(oldStatus, &contact.Status) {
		if err := r.Client.Status().Patch(ctx, contact, client.MergeFrom(original), client.FieldOwner("loopscontact-controller")); err != nil {
			log.Error(err, "Failed to patch contact status")
			return ctrl.Result{}, fmt.Errorf("failed to patch contact status: %w", err)
		}
	} else {
		log.Info("Contact status unchanged, skipping update")
	}

	log.Info("Contact reconciled")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LoopsContactController) SetupWithManager(mgr ctrl.Manager) error {
	// Register finalizer
	r.Finalizers = finalizer.NewFinalizers()
	if err := r.Finalizers.Register(loopsContactFinalizerKey, &loopsContactFinalizer{
		Client: r.Client,
		Loops:  r.Loops,
	}); err != nil {
		return fmt.Errorf("failed to register loops contact finalizer: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&notificationmiloapiscomv1alpha1.Contact{}).
		Named("contact").
		Complete(r)
}

func (r *LoopsContactController) createContact(ctx context.Context, contact *notificationmiloapiscomv1alpha1.Contact) (emailprovider.LoopsCreateResponse, error) {
	log := logf.FromContext(ctx).WithValues("controller", "LoopsContactController", "trigger", contact.Name)
	log.Info("Creating Loops contact")

	// Get Loops contact
	existing, err := r.Loops.FindContactByUserID(ctx, string(contact.UID))
	if err != nil {
		log.Error(err, "Failed to find Loops contact")
		return emailprovider.LoopsCreateResponse{}, fmt.Errorf("failed to find Loops contact: %w", err)
	}
	if len(existing) > 0 {
		log.Info("Loops contact already exists")
		return emailprovider.LoopsCreateResponse{
			Success: true,
			ID:      existing[0].ID,
		}, nil
	}

	// Create Loops contact
	created, err := r.Loops.CreateContact(ctx, contact.Spec.Email, contact.Spec.GivenName, contact.Spec.FamilyName, string(contact.UID))
	if err != nil {
		log.Error(err, "Failed to create Loops contact")
		return emailprovider.LoopsCreateResponse{}, fmt.Errorf("failed to create Loops contact: %w", err)
	}

	return created, nil
}

func (r *LoopsContactController) updateContact(ctx context.Context, contact *notificationmiloapiscomv1alpha1.Contact) error {
	log := logf.FromContext(ctx).WithValues("controller", "LoopsContactController", "trigger", contact.Name)
	log.Info("Updating Loops contact")

	// Get Loops contact
	existing, err := r.Loops.FindContactByUserID(ctx, string(contact.UID))
	if err != nil {
		log.Error(err, "Failed to find Loops contact")
		return fmt.Errorf("failed to find Loops contact: %w", err)
	}

	if len(existing) == 0 {
		log.Info("Loops contact not found")
		return fmt.Errorf("loops contact not found")
	}

	// Update Loops contact
	_, err = r.Loops.UpdateContact(ctx, contact.Spec.Email, contact.Spec.GivenName, contact.Spec.FamilyName, string(contact.UID), nil)
	if err != nil {
		log.Error(err, "Failed to update Loops contact")
		return fmt.Errorf("failed to update Loops contact: %w", err)
	}

	return nil
}

func (f *loopsContactFinalizer) DeleteContact(ctx context.Context, contact *notificationmiloapiscomv1alpha1.Contact) error {
	log := logf.FromContext(ctx).WithValues("controller", "LoopsContactController", "trigger", contact.Name)
	log.Info("Deleting Loops contact")

	// Delete Loops contact
	_, err := f.Loops.DeleteContact(ctx, string(contact.UID))
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete Loops contact")
		return fmt.Errorf("failed to delete Loops contact: %w", err)
	}

	return nil
}