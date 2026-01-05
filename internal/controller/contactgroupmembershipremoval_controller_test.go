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

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
	notificationv1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = ginkgo.Describe("ContactGroupMembershipRemovalController", func() {
	var (
		ctx        context.Context
		k8sClient  client.Client
		controller *ContactGroupMembershipRemovalController

		contact      *notificationv1.Contact
		contactGroup *notificationv1.ContactGroup
		membership   *notificationv1.ContactGroupMembership
		removal      *notificationv1.ContactGroupMembershipRemoval
	)

	ginkgo.BeforeEach(func() {
		ctx = context.Background()

		// Base objects
		contact = &notificationv1.Contact{
			TypeMeta:   metav1.TypeMeta{APIVersion: "notification.miloapis.com/v1alpha1", Kind: "Contact"},
			ObjectMeta: metav1.ObjectMeta{Name: "john-doe", Namespace: "default"},
			Spec: notificationv1.ContactSpec{
				FamilyName: "Doe",
				GivenName:  "John",
				Email:      "john@example.com",
				SubjectRef: &notificationv1.SubjectReference{Kind: "User", Name: "john-user"},
			},
		}

		contactGroup = &notificationv1.ContactGroup{
			TypeMeta:   metav1.TypeMeta{APIVersion: "notification.miloapis.com/v1alpha1", Kind: "ContactGroup"},
			ObjectMeta: metav1.ObjectMeta{Name: "group-1", Namespace: "default"},
			Spec:       notificationv1.ContactGroupSpec{DisplayName: "Group 1"},
		}

		membership = &notificationv1.ContactGroupMembership{
			TypeMeta:   metav1.TypeMeta{APIVersion: "notification.miloapis.com/v1alpha1", Kind: "ContactGroupMembership"},
			ObjectMeta: metav1.ObjectMeta{Name: "member-1", Namespace: "default"},
			Spec: notificationv1.ContactGroupMembershipSpec{
				ContactRef:      notificationv1.ContactReference{Name: contact.Name, Namespace: contact.Namespace},
				ContactGroupRef: notificationv1.ContactGroupReference{Name: contactGroup.Name, Namespace: contactGroup.Namespace},
			},
		}

		removal = &notificationv1.ContactGroupMembershipRemoval{
			TypeMeta:   metav1.TypeMeta{APIVersion: "notification.miloapis.com/v1alpha1", Kind: "ContactGroupMembershipRemoval"},
			ObjectMeta: metav1.ObjectMeta{Name: "remove-1", Namespace: "default"},
			Spec: notificationv1.ContactGroupMembershipRemovalSpec{
				ContactRef:      notificationv1.ContactReference{Name: contact.Name, Namespace: contact.Namespace},
				ContactGroupRef: notificationv1.ContactGroupReference{Name: contactGroup.Name, Namespace: contactGroup.Namespace},
			},
		}

		sch := scheme.Scheme
		gomega.Expect(notificationv1.AddToScheme(sch)).To(gomega.Succeed())

		// Build fake client with status subresources for CGMR and Membership
		k8sClient = fake.NewClientBuilder().
			WithScheme(sch).
			WithStatusSubresource(&notificationv1.ContactGroupMembershipRemoval{}, &notificationv1.ContactGroupMembership{}).
			WithObjects(contact.DeepCopy(), contactGroup.DeepCopy(), membership.DeepCopy(), removal.DeepCopy()).
			WithIndex(&notificationv1.ContactGroupMembership{}, contactAndContactGroupTupleIndexKey, func(raw client.Object) []string {
				c := raw.(*notificationv1.ContactGroupMembership)
				return []string{buildContactAndContactGroupTupleIndexKey(c.Spec.ContactRef, c.Spec.ContactGroupRef)}
			}).
			Build()

		controller = &ContactGroupMembershipRemovalController{Client: k8sClient}
	})

	ginkgo.Context("Reconcile", func() {
		ginkgo.It("deletes matching memberships and sets Ready condition", func() {
			// Invoke reconcile
			_, err := controller.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: removal.Name, Namespace: removal.Namespace}})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Membership should be gone
			list := &notificationv1.ContactGroupMembershipList{}
			gomega.Expect(k8sClient.List(ctx, list)).To(gomega.Succeed())
			gomega.Expect(list.Items).To(gomega.BeEmpty())

			// Removal object should have Ready condition true
			fetched := &notificationv1.ContactGroupMembershipRemoval{}
			gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: removal.Name, Namespace: removal.Namespace}, fetched)).To(gomega.Succeed())
			// Check status.username is set from contact's SubjectRef
			gomega.Expect(fetched.Status.Username).To(gomega.Equal("john-user"))
			cond := meta.FindStatusCondition(fetched.Status.Conditions, notificationv1.ContactGroupMembershipRemovalReadyCondition)
			gomega.Expect(cond).NotTo(gomega.BeNil())
			gomega.Expect(cond.Status).To(gomega.Equal(metav1.ConditionTrue))
		})

		ginkgo.It("skips if Ready condition already true and Username is set", func() {
			// Pre-set Ready condition and Username on latest version of the object
			latest := &notificationv1.ContactGroupMembershipRemoval{}
			gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: removal.Name, Namespace: removal.Namespace}, latest)).To(gomega.Succeed())

			latest.Status.Username = "john-user"
			meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
				Type:   notificationv1.ContactGroupMembershipRemovalReadyCondition,
				Status: metav1.ConditionTrue,
			})
			gomega.Expect(k8sClient.Status().Update(ctx, latest)).To(gomega.Succeed())

			// Reconcile
			_, err := controller.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: removal.Name, Namespace: removal.Namespace}})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Membership remains (still one because reconcile skipped)
			list := &notificationv1.ContactGroupMembershipList{}
			gomega.Expect(k8sClient.List(ctx, list)).To(gomega.Succeed())
			gomega.Expect(list.Items).To(gomega.HaveLen(1))
		})
	})

})
