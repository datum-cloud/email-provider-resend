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
	"strings"

	"go.miloapis.com/email-provider-resend/internal/emailprovider"
	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/finalizer"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	loopsContactFinalizerKey = "notification.miloapis.com/loops-contact"
	loopsContactIndexKey     = "loopscontact-index"
)

const (
	// NewsLetterAddedCondition is a condition that is set to true when the mailing list is added to the Loops contact
	NewsLetterAddedCondition = "NewsLetterAdded"
	// NewsLetterAddedReason is a reason that is set when the mailing list is added to the Loops contact
	NewsLetterAddedReason = "NewsLetterAdded"
	// NewsLetterNotAddedReason is a reason that is set when the mailing list is not added to the Loops contact
	NewsLetterNotAddedReason = "NewsLetterNotAdded"
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
	Client                   client.Client
	Finalizers               finalizer.Finalizers
	Loops                    emailprovider.LoopsEmail
	NewsLetterListId         string
	NewsletterContactGroupId string
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
			if errors.IsConflict(updateErr) {
				log.Info("Conflict updating Contact after finalizer update; requeuing")
				return ctrl.Result{Requeue: true}, nil
			}
			log.Error(updateErr, "Failed to update Contact after finalizer update")
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, nil
	}

	oldStatus := contact.Status.DeepCopy()
	original := contact.DeepCopy()
	readyCond := meta.FindStatusCondition(contact.Status.Conditions, LoopsContactReadyCondition)

	switch {
	// First creation – condition not present yet
	case readyCond == nil || readyCond.Reason == LoopsContactNotCreatedReason:
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
		if err != nil {
			if errors.IsConflict(err) || errors.IsBadRequest(err) || errors.IsNotFound(err) {
				log.Info("Failed to update contact on email provider", "error", err.Error())
				meta.SetStatusCondition(&contact.Status.Conditions, metav1.Condition{
					Type:               LoopsContactReadyCondition,
					Status:             metav1.ConditionFalse,
					Reason:             LoopsContactNotUpdatedReason,
					Message:            fmt.Sprintf("Loops contact not updated on email provider: %s", err.Error()),
					LastTransitionTime: metav1.Now(),
					ObservedGeneration: contact.GetGeneration(),
				})
			} else {
				log.Error(err, "Failed to update Loops contact")
				return ctrl.Result{}, fmt.Errorf("failed to update Loops contact: %w", err)
			}
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

	errorAddingToNewsLetter := false
	if r.isNewsletterContact(contact) {
		errorAddingToNewsLetter = r.addToNewsLetterList(ctx, contact)
	} else {
		errorAddingToNewsLetter = r.addToNewsletterIfInNewsletterContactGroupMembership(ctx, contact)
	}

	// Update contact status if it changed
	if !equality.Semantic.DeepEqual(oldStatus, &contact.Status) {
		if err := r.Client.Status().Patch(ctx, contact, client.MergeFrom(original), client.FieldOwner("loopscontact-controller")); err != nil {
			log.Error(err, "Failed to patch contact status")
			return ctrl.Result{}, fmt.Errorf("failed to patch contact status: %w", err)
		}
	} else {
		log.Info("Contact status unchanged, skipping update")
	}

	if errorAddingToNewsLetter {
		log.Error(errors.NewInternalError(fmt.Errorf("failed to add mailing list to Loops contact")), "Failed to add mailing list to Loops contact")
		return ctrl.Result{}, fmt.Errorf("failed to add mailing list to Loops contact")
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

	// Index by contact group membership for efficient contact group membership lookup
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &notificationmiloapiscomv1alpha1.ContactGroupMembership{}, loopsContactIndexKey, func(rawObj client.Object) []string {
		cgm := rawObj.(*notificationmiloapiscomv1alpha1.ContactGroupMembership)
		return []string{buildContactNamespacedIndexKey(cgm.Spec.ContactRef.Name, cgm.Spec.ContactRef.Namespace)}
	}); err != nil {
		return fmt.Errorf("failed to index contactgroupmembership by contact name: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&notificationmiloapiscomv1alpha1.Contact{}).
		// Watch ContactGroupMembership changes and enqueue reconcile requests for referenced Contacts
		WatchesRawSource(
			source.Kind(
				mgr.GetCache(),
				&notificationmiloapiscomv1alpha1.ContactGroupMembership{},
				handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, cgm *notificationmiloapiscomv1alpha1.ContactGroupMembership) []reconcile.Request {
					if cgm.Spec.ContactRef.Name == "" || cgm.Spec.ContactRef.Namespace == "" {
						return nil
					}
					return []reconcile.Request{
						{NamespacedName: types.NamespacedName{
							Name:      cgm.Spec.ContactRef.Name,
							Namespace: cgm.Spec.ContactRef.Namespace,
						}},
					}
				}),
			),
		).
		Named("loopscontact").
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
		return errors.NewNotFound(
			schema.GroupResource{Group: "loops", Resource: "contacts"},
			string(contact.UID),
		)
	}

	// Update Loops contact
	_, err = r.Loops.UpdateContact(ctx, contact.Spec.Email, contact.Spec.GivenName, contact.Spec.FamilyName, string(contact.UID), nil)
	if err != nil {
		log.Error(err, "Failed to update Loops contact")
		return err
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

func (r *LoopsContactController) addToNewsLetterList(ctx context.Context, contact *notificationmiloapiscomv1alpha1.Contact) bool {
	log := logf.FromContext(ctx).WithValues("controller", "LoopsContactController", "trigger", contact.Name)
	log.Info("Adding mailing list to Loops contact")

	newsLetterCond := meta.FindStatusCondition(contact.Status.Conditions, NewsLetterAddedCondition)
	if newsLetterCond != nil && newsLetterCond.Status == metav1.ConditionTrue {
		log.Info("News letter already added")
		return false
	}

	// Add mailing list to Loops contact
	_, err := r.Loops.UpdateContactMailingLists(ctx, string(contact.UID), []emailprovider.LoopsMailingList{{ID: r.NewsLetterListId, Subscribed: true}})
	if err != nil {
		log.Error(err, "Failed to add mailing list to Loops contact")

		meta.SetStatusCondition(&contact.Status.Conditions, metav1.Condition{
			Type:               NewsLetterAddedCondition,
			Status:             metav1.ConditionFalse,
			Reason:             NewsLetterNotAddedReason,
			Message:            fmt.Sprintf("Contact not added to Newsletter list: %s", err.Error()),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: contact.GetGeneration(),
		})

		return true
	}

	meta.SetStatusCondition(&contact.Status.Conditions, metav1.Condition{
		Type:               NewsLetterAddedCondition,
		Status:             metav1.ConditionTrue,
		Reason:             NewsLetterAddedReason,
		Message:            "Contact added to Newsletter list on email provider.",
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: contact.GetGeneration(),
	})

	return false
}

// isNewsletterContact returns true if the contact name starts with "newsletter-".
func (r *LoopsContactController) isNewsletterContact(contact *notificationmiloapiscomv1alpha1.Contact) bool {
	return strings.HasPrefix(contact.Name, "newsletter-")
}

func (r *LoopsContactController) addToNewsletterIfInNewsletterContactGroupMembership(ctx context.Context, contact *notificationmiloapiscomv1alpha1.Contact) bool {
	log := logf.FromContext(ctx).WithValues("controller", "LoopsContactController", "trigger", contact.Name)
	log.Info("Checking if contact is in newsletter contact group membership")

	contactGroupMembershipList := &notificationmiloapiscomv1alpha1.ContactGroupMembershipList{}
	err := r.Client.List(ctx, contactGroupMembershipList, client.MatchingFields{loopsContactIndexKey: buildContactNamespacedIndexKey(contact.Name, contact.Namespace)})
	if err != nil {
		log.Error(err, "Failed to list contact group memberships")
		return true
	}

	for _, contactGroupMembership := range contactGroupMembershipList.Items {
		if contactGroupMembership.Spec.ContactGroupRef.Name == r.NewsletterContactGroupId {
			return r.addToNewsLetterList(ctx, contact)
		}
	}

	return false
}
