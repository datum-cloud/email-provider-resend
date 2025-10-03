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

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	contactGroupMembershipFinalizerKey = "notification.miloapis.com/contactgroupmembership"
	cgmByContactNamespacedNameIndexKey = "cgm-by-contact-namespaced-name-index"
)

// ContactGroupMembershipReconciler reconciles a ContactGroupMembership object
type ContactGroupMembershipController struct {
	Client        client.Client
	EmailProvider emailprovider.Service
	Finalizers    finalizer.Finalizers
}

type contactGroupMembershipFinalizer struct {
	Client        client.Client
	EmailProvider emailprovider.Service
}

// Finalize is the finalizer for the ContactGroupMembership object
func (f *contactGroupMembershipFinalizer) Finalize(ctx context.Context, obj client.Object) (finalizer.Result, error) {
	log := logf.FromContext(ctx).WithValues("finalizer", "ContactGroupMembershipFinalizer", "trigger", obj.GetName())
	log.Info("Finalizing ContactGroupMembership")

	// Type assertion
	contactGroupMembership, ok := obj.(*notificationmiloapiscomv1alpha1.ContactGroupMembership)
	if !ok {
		log.Error(fmt.Errorf("object is not a ContactGroupMembership"), "Failed to finalize ContactGroupMembership")
		return finalizer.Result{}, fmt.Errorf("object is not a ContactGroupMembership")
	}

	if meta.IsStatusConditionTrue(contactGroupMembership.Status.Conditions, notificationmiloapiscomv1alpha1.ContactGroupMembershipDeletedCondition) {
		log.Info("ContactGroupMembership deletion confirmed by email provider. ContactGroupMembership finalizer completed.")
		return finalizer.Result{}, nil
	}

	// Get Referenced ContactGroup
	contactGroup := &notificationmiloapiscomv1alpha1.ContactGroup{}
	err := f.Client.Get(ctx, client.ObjectKey{Name: contactGroupMembership.Spec.ContactGroupRef.Name, Namespace: contactGroupMembership.Spec.ContactGroupRef.Namespace}, contactGroup)
	if err != nil {
		log.Error(err, "Failed to get ContactGroup")
		return finalizer.Result{}, fmt.Errorf("failed to get ContactGroup: %w", err)
	}

	// Delete ContactGroupMembership from email provider
	delResult, err := f.EmailProvider.DeleteContactGroupMembershipIdempotent(ctx, *contactGroupMembership, *contactGroup)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("ContactGroupMembership not found on email provider. Probably deleted. ContactGroupMembership finalizer completed.")
			return finalizer.Result{}, nil
		}
		log.Error(err, "Failed to delete ContactGroupMembership from email provider")
		return finalizer.Result{}, fmt.Errorf("failed to delete ContactGroupMembership from email provider: %w", err)
	}
	if !delResult.Deleted {
		log.Error(fmt.Errorf("failed to delete ContactGroupMembership from email provider. Expected deleted to be true, got %t", delResult.Deleted), "Failed to delete ContactGroupMembership from email provider")
		return finalizer.Result{}, fmt.Errorf("failed to delete ContactGroupMembership from email provider. Expected deleted to be true, got %t", delResult.Deleted)
	} else {
		// Update ContactGroupMembership status to pending deletion on email provider
		meta.SetStatusCondition(&contactGroupMembership.Status.Conditions, metav1.Condition{
			Type:               notificationmiloapiscomv1alpha1.ContactGroupMembershipDeletedCondition,
			Status:             metav1.ConditionFalse,
			Reason:             notificationmiloapiscomv1alpha1.ContactGroupMembershipDeletePendingReason,
			Message:            "ContactGroupMembership delete pending, waiting for email provider confirmation webhook to be triggered",
			LastTransitionTime: metav1.Now(),
		})
		if err := f.Client.Status().Update(ctx, contactGroupMembership); err != nil {
			log.Error(err, "Failed to update ContactGroupMembership status")
			return finalizer.Result{}, fmt.Errorf("failed to update ContactGroupMembership status: %w", err)
		}

	}

	log.Info("Waiting for email provider confirmation webhook to be triggered for ContactGroupMembership deletion")
	// We just return the error, as we need to wait for the email provider confirmation webhook to be triggered
	// Getting the resource again would not help for validation purposes will fail, as the webhook takes some time to trigger
	return finalizer.Result{}, fmt.Errorf("waiting for email provider confirmation webhook to be triggered for ContactGroupMembership deletion")
}

// +kubebuilder:rbac:groups=notification.miloapis.com,resources=contactgroupmemberships,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups=notification.miloapis.com,resources=contactgroupmemberships/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=notification.miloapis.com,resources=contactgroupmemberships/finalizers,verbs=update
// +kubebuilder:rbac:groups=notification.miloapis.com,resources=contactgroupmembershipremovals,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=notification.miloapis.com,resources=contacts,verbs=get
// +kubebuilder:rbac:groups=notification.miloapis.com,resources=contactgroups,verbs=get

// Reconcile is the main function that reconciles the ContactGroupMembership object
func (r *ContactGroupMembershipController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("controller", "ContactGroupMembershipController", "trigger", req.NamespacedName)
	log.Info("Starting reconciliation", "namespacedName", req.String(), "name", req.Name, "namespace", req.Namespace)

	// Get ContactGroupMembership
	contactGroupMembership := &notificationmiloapiscomv1alpha1.ContactGroupMembership{}
	err := r.Client.Get(ctx, req.NamespacedName, contactGroupMembership)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("ContactGroupMembership not found. Probably deleted.")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get ContactGroupMembership")
		return ctrl.Result{}, fmt.Errorf("failed to get ContactGroupMembership: %w", err)
	}

	// Run finalizers
	finalizeResult, err := r.Finalizers.Finalize(ctx, contactGroupMembership)
	if err != nil {
		log.Error(err, "Failed to run finalizers for ContactGroupMembership")
		return ctrl.Result{}, fmt.Errorf("failed to run finalizers for ContactGroupMembership: %w", err)
	}
	if finalizeResult.Updated {
		log.Info("finalizer updated the contact group membership object, updating API server")
		if updateErr := r.Client.Update(ctx, contactGroupMembership); updateErr != nil {
			log.Error(updateErr, "Failed to update ContactGroupMembership after finalizer update")
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, nil
	}

	// Get Referenced Contact
	contact := &notificationmiloapiscomv1alpha1.Contact{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: contactGroupMembership.Spec.ContactRef.Name, Namespace: contactGroupMembership.Spec.ContactRef.Namespace}, contact)
	if err != nil {
		log.Error(err, "Failed to get Contact")
		return ctrl.Result{}, fmt.Errorf("failed to get Contact: %w", err)
	}

	// Get Referenced ContactGroup
	contactGroup := &notificationmiloapiscomv1alpha1.ContactGroup{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: contactGroupMembership.Spec.ContactGroupRef.Name, Namespace: contactGroupMembership.Spec.ContactGroupRef.Namespace}, contactGroup)
	if err != nil {
		log.Error(err, "Failed to get ContactGroup")
		return ctrl.Result{}, fmt.Errorf("failed to get ContactGroup: %w", err)
	}

	oldStatus := contactGroupMembership.Status.DeepCopy()
	existingCond := meta.FindStatusCondition(contactGroupMembership.Status.Conditions, notificationmiloapiscomv1alpha1.ContactGroupMembershipReadyCondition)
	updatedCond := meta.FindStatusCondition(contactGroupMembership.Status.Conditions, notificationmiloapiscomv1alpha1.ContactGroupMembershipUpdatedCondition)

	switch {
	case existingCond == nil:
		log.Info("ContactGroupMembership first creation")
		// First creation – condition not present yet
		// Create ContactGroupMembership on email provider
		emailProviderContactGroupMembership, err := r.EmailProvider.CreateContactGroupMembershipIdempotent(ctx, *contactGroup, *contact)
		if err != nil {
			log.Error(err, "Failed to create ContactGroupMembership on email provider")
			return ctrl.Result{}, fmt.Errorf("failed to create ContactGroupMembership on email provider: %w", err)
		}
		contactGroupMembership.Status.ProviderID = emailProviderContactGroupMembership.ContactGroupMembershipID
		meta.SetStatusCondition(&contactGroupMembership.Status.Conditions, metav1.Condition{
			Type:               notificationmiloapiscomv1alpha1.ContactGroupMembershipReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             notificationmiloapiscomv1alpha1.ContactGroupMembershipCreatePendingReason,
			Message:            "ContactGroupMembership create pending, waiting for email provider confirmation webhook to be triggered",
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: contactGroupMembership.GetGeneration(),
		})

	// Update requested – Updated condition is False with the specific reason
	case updatedCond != nil && updatedCond.Status == metav1.ConditionFalse && updatedCond.Reason == notificationmiloapiscomv1alpha1.ContactGroupMembershipUpdateRequestedReason:
		log.Info("ContactGroupMembership update requested")
		// Update ContactGroupMembership on email provider
		emailProviderContactGroupMembership, err := r.EmailProvider.UpdateContactGroupMembership(ctx, *contactGroupMembership, *contactGroup, *contact)
		if err != nil {
			log.Error(err, "Failed to update ContactGroupMembership on email provider")
			return ctrl.Result{}, fmt.Errorf("failed to update ContactGroupMembership on email provider: %w", err)
		}
		contactGroupMembership.Status.ProviderID = emailProviderContactGroupMembership.ContactGroupMembershipID

		// Update ContactGroupMembership status to pending. This will avoid multiple updates to the email provider.
		meta.SetStatusCondition(&contactGroupMembership.Status.Conditions, metav1.Condition{
			Type:               notificationmiloapiscomv1alpha1.ContactGroupMembershipUpdatedCondition,
			Status:             metav1.ConditionFalse,
			Reason:             notificationmiloapiscomv1alpha1.ContactGroupMembershipUpdatePendingReason,
			Message:            "ContactGroupMembership update pending, waiting for email provider confirmation webhook to be triggered",
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: contactGroupMembership.GetGeneration(),
		})
	}

	// Update status if it changed
	if !equality.Semantic.DeepEqual(oldStatus, &contactGroupMembership.Status) {
		if err := r.Client.Status().Update(ctx, contactGroupMembership); err != nil {
			log.Error(err, "Failed to update contact group membership status")
			return ctrl.Result{}, fmt.Errorf("failed to update contact group membership status: %w", err)
		}
	} else {
		log.Info("Contact group membership status unchanged, skipping update")
	}

	log.Info("Contact group membership reconciled")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ContactGroupMembershipController) SetupWithManager(mgr ctrl.Manager) error {
	// Index by contact ref and contact group ref for efficient contact group membership removal lookup
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &notificationmiloapiscomv1alpha1.ContactGroupMembershipRemoval{}, contactAndContactGroupTupleIndexKey, func(rawObj client.Object) []string {
		cgmr := rawObj.(*notificationmiloapiscomv1alpha1.ContactGroupMembershipRemoval)
		return []string{buildContactAndContactGroupTupleIndexKey(cgmr.Spec.ContactRef, cgmr.Spec.ContactGroupRef)}
	}); err != nil {
		return fmt.Errorf("failed to index contactgroupmembership by contact ref and contact group ref: %w", err)
	}
	// Index by contact namespaced key for efficient lookup from contact updates
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &notificationmiloapiscomv1alpha1.ContactGroupMembership{}, cgmByContactNamespacedNameIndexKey, func(rawObj client.Object) []string {
		cgm := rawObj.(*notificationmiloapiscomv1alpha1.ContactGroupMembership)
		return []string{buildContactNamespacedIndexKey(cgm.Spec.ContactRef.Name, cgm.Spec.ContactRef.Namespace)}
	}); err != nil {
		return fmt.Errorf("failed to index contactgroupmembership by contact name: %w", err)
	}

	r.Finalizers = finalizer.NewFinalizers()
	if err := r.Finalizers.Register(contactGroupMembershipFinalizerKey, &contactGroupMembershipFinalizer{
		Client:        r.Client,
		EmailProvider: r.EmailProvider}); err != nil {
		return fmt.Errorf("failed to register contact group membership finalizer: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&notificationmiloapiscomv1alpha1.ContactGroupMembership{}).
		Watches(
			&notificationmiloapiscomv1alpha1.Contact{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueContactForContactGroupMembershipUpdate),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc:  func(e event.CreateEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return false },
				GenericFunc: func(e event.GenericEvent) bool { return false },
				UpdateFunc:  func(e event.UpdateEvent) bool { return true },
			}),
		).
		Named("contactgroupmembership").
		Complete(r)
}

func (r *ContactGroupMembershipController) enqueueContactForContactGroupMembershipUpdate(ctx context.Context, obj client.Object) []reconcile.Request {
	log := logf.FromContext(ctx).WithValues("controller", "enqueueContactForContactGroupMembershipUpdate", "trigger", obj.GetName())

	contact, ok := obj.(*notificationmiloapiscomv1alpha1.Contact)
	if !ok {
		log.Error(fmt.Errorf("object is not a Contact"), "Failed to enqueue ContactGroupMembership for update")
		return nil
	}

	cgmList := &notificationmiloapiscomv1alpha1.ContactGroupMembershipList{}
	err := r.Client.List(ctx, cgmList, client.MatchingFields{cgmByContactNamespacedNameIndexKey: buildContactNamespacedIndexKey(contact.Name, contact.Namespace)})
	if err != nil {
		log.Error(err, "Failed to list ContactGroupMembership")
		return nil
	}

	var reqs = make([]reconcile.Request, 0, len(cgmList.Items))
	for _, cgm := range cgmList.Items {
		reqs = append(reqs, ctrl.Request{
			NamespacedName: client.ObjectKey{Name: cgm.Name, Namespace: cgm.Namespace},
		})
	}

	return reqs
}
