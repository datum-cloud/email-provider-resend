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

	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	contactAndContactGroupTupleIndexKey = "contact-and-contactgroup-tuple-index"
)

// buildContactAndContactGroupTupleIndexKey returns "<contact-ns>|<contact-name>|<contactgroup-ns>|<contactgroup-name>"
func buildContactAndContactGroupTupleIndexKey(contactRef notificationmiloapiscomv1alpha1.ContactReference, contactGroupRef notificationmiloapiscomv1alpha1.ContactGroupReference) string {
	return fmt.Sprintf("%s|%s|%s|%s", contactRef.Namespace, contactRef.Name, contactGroupRef.Namespace, contactGroupRef.Name)
}

// ContactGroupMembershipRemovalReconciler reconciles a ContactGroupMembershipRemoval object
type ContactGroupMembershipRemovalController struct {
	Client client.Client
}

// +kubebuilder:rbac:groups=notification.miloapis.com,resources=contactgroupmembershipremovals,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=notification.miloapis.com,resources=contactgroupmembershipremovals/status,verbs=update
// +kubebuilder:rbac:groups=notification.miloapis.com,resources=contactgroupmemberships,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=notification.miloapis.com,resources=contacts,verbs=get
// +kubebuilder:rbac:groups=notification.miloapis.com,resources=contactgroups,verbs=get
// Reconcile is the main function that reconciles the ContactGroupMembershipRemoval object
func (r *ContactGroupMembershipRemovalController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("controller", "ContactGroupMembershipRemovalController", "trigger", req.NamespacedName)
	log.Info("Starting reconciliation", "namespacedName", req.String(), "name", req.Name, "namespace", req.Namespace)

	// Get ContactGroupMembershipRemoval
	cgmr := &notificationmiloapiscomv1alpha1.ContactGroupMembershipRemoval{}
	if err := r.Client.Get(ctx, req.NamespacedName, cgmr); err != nil {
		if errors.IsNotFound(err) {
			log.Info("ContactGroupMembershipRemoval not found. Probably deleted.")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get ContactGroupMembershipRemoval")
		return ctrl.Result{}, fmt.Errorf("failed to get ContactGroupMembershipRemoval: %w", err)
	}

	if meta.IsStatusConditionTrue(cgmr.Status.Conditions, notificationmiloapiscomv1alpha1.ContactGroupMembershipRemovalReadyCondition) {
		log.Info("Skipping ContactGroupMembershipRemoval reconciliation, as it is already reconciled")
		return ctrl.Result{}, nil
	}

	// Get associated ContactGroupMemberships to contact group name and contact ref
	contactGroupMemberships := &notificationmiloapiscomv1alpha1.ContactGroupMembershipList{}
	err := r.Client.List(
		ctx,
		contactGroupMemberships, client.MatchingFields{contactAndContactGroupTupleIndexKey: buildContactAndContactGroupTupleIndexKey(cgmr.Spec.ContactRef, cgmr.Spec.ContactGroupRef)})
	if err != nil {
		log.Error(err, "Failed to list ContactGroupMemberships")
		return ctrl.Result{}, fmt.Errorf("failed to list ContactGroupMemberships: %w", err)
	}

	// Delete associated ContactGroupMembership
	for _, cgm := range contactGroupMemberships.Items {
		if err := r.Client.Delete(ctx, &cgm); err != nil {
			if errors.IsNotFound(err) {
				log.Info("ContactGroupMembership not found. Probably deleted.")
				continue
			}
			log.Error(err, "Failed to delete ContactGroupMembership")
			return ctrl.Result{}, fmt.Errorf("failed to delete ContactGroupMembership: %w", err)
		}
	}

	meta.SetStatusCondition(&cgmr.Status.Conditions, metav1.Condition{
		Type:               notificationmiloapiscomv1alpha1.ContactGroupMembershipRemovalReadyCondition,
		Status:             metav1.ConditionTrue,
		Reason:             notificationmiloapiscomv1alpha1.ContactGroupMembershipRemovalCreatedReason,
		Message:            "Contact Group Membership Removal completed",
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: cgmr.GetGeneration(),
	})
	if err := r.Client.Status().Update(ctx, cgmr); err != nil {
		log.Error(err, "Failed to update ContactGroupMembershipRemoval status")
		return ctrl.Result{}, fmt.Errorf("failed to update ContactGroupMembershipRemoval status: %w", err)
	}

	log.Info("ContactGroupMembershipRemoval reconciled")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ContactGroupMembershipRemovalController) SetupWithManager(mgr ctrl.Manager) error {
	// Index by contact ref and contact group ref for efficient contact group membership lookup
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &notificationmiloapiscomv1alpha1.ContactGroupMembership{}, contactAndContactGroupTupleIndexKey, func(rawObj client.Object) []string {
		cgm := rawObj.(*notificationmiloapiscomv1alpha1.ContactGroupMembership)
		return []string{buildContactAndContactGroupTupleIndexKey(cgm.Spec.ContactRef, cgm.Spec.ContactGroupRef)}
	}); err != nil {
		return fmt.Errorf("failed to index contactgroupmembership by contact ref and contact group ref: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&notificationmiloapiscomv1alpha1.ContactGroupMembershipRemoval{}).
		Named("contactgroupmembershipremoval").
		Complete(r)
}
