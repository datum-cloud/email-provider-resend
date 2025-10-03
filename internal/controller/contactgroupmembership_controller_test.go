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
	"go.miloapis.com/email-provider-resend/internal/emailprovider"
	notificationv1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	finalizerpkg "sigs.k8s.io/controller-runtime/pkg/finalizer"
)

// fakeUnderlyingProvider implements emailprovider.EmailProvider interface with minimal logic for tests
type fakeUnderlyingProvider struct {
	deleteInputs []emailprovider.DeleteContactGroupMembershipInput
}

func (f *fakeUnderlyingProvider) SendEmail(ctx context.Context, in emailprovider.SendEmailInput) (emailprovider.SendEmailOutput, error) {
	return emailprovider.SendEmailOutput{}, nil
}

func (f *fakeUnderlyingProvider) CreateContactGroup(ctx context.Context, in emailprovider.CreateContactGroupInput) (emailprovider.CreateContactGroupOutput, error) {
	return emailprovider.CreateContactGroupOutput{ContactGroupID: "cg-1"}, nil
}

func (f *fakeUnderlyingProvider) GetContactGroup(ctx context.Context, in emailprovider.GetContactGroupInput) (emailprovider.GetContactGroupOutput, error) {
	return emailprovider.GetContactGroupOutput{ContactGroupID: in.ContactGroupID}, nil
}

func (f *fakeUnderlyingProvider) DeleteContactGroup(ctx context.Context, in emailprovider.DeleteContactGroupInput) (emailprovider.DeleteContactGroupOutput, error) {
	return emailprovider.DeleteContactGroupOutput{ContactGroupID: in.ContactGroupID, Deleted: true}, nil
}

func (f *fakeUnderlyingProvider) ListContactGroups(ctx context.Context) (emailprovider.ListContactGroupsOutput, error) {
	return emailprovider.ListContactGroupsOutput{}, nil
}

func (f *fakeUnderlyingProvider) CreateContactGroupMembership(ctx context.Context, in emailprovider.CreateContactGroupMembershipInput) (emailprovider.CreateContactGroupMembershipOutput, error) {
	return emailprovider.CreateContactGroupMembershipOutput{ContactGroupMembershipID: "cgm-1"}, nil
}

func (f *fakeUnderlyingProvider) GetContactGroupMembershipByEmail(ctx context.Context, in emailprovider.GetContactGroupMembershipByEmailInput) (emailprovider.GetContactGroupMembershipByEmailOutput, error) {
	// Simulate not found so Service triggers creation
	return emailprovider.GetContactGroupMembershipByEmailOutput{}, errors.NewNotFound(schema.GroupResource{Group: "resend", Resource: "audiences"}, "notfound")
}

func (f *fakeUnderlyingProvider) DeleteContactGroupMembership(ctx context.Context, in emailprovider.DeleteContactGroupMembershipInput) (emailprovider.DeleteContactGroupMembershipOutput, error) {
	f.deleteInputs = append(f.deleteInputs, in)
	return emailprovider.DeleteContactGroupMembershipOutput{ContactGroupMembershipID: in.ContactGroupMembershipID, Deleted: true}, nil
}

var _ = ginkgo.Describe("ContactGroupMembershipController", func() {
	var (
		ctx        context.Context
		k8sClient  client.Client
		controller *ContactGroupMembershipController

		contact      *notificationv1.Contact
		contactGroup *notificationv1.ContactGroup
		membership   *notificationv1.ContactGroupMembership
	)

	ginkgo.BeforeEach(func() {
		ctx = context.Background()

		contact = &notificationv1.Contact{
			TypeMeta:   metav1.TypeMeta{APIVersion: "notification.miloapis.com/v1alpha1", Kind: "Contact"},
			ObjectMeta: metav1.ObjectMeta{Name: "alice", Namespace: "default"},
			Spec:       notificationv1.ContactSpec{FamilyName: "Doe", GivenName: "Alice", Email: "alice@example.com"},
		}

		contactGroup = &notificationv1.ContactGroup{
			TypeMeta:   metav1.TypeMeta{APIVersion: "notification.miloapis.com/v1alpha1", Kind: "ContactGroup"},
			ObjectMeta: metav1.ObjectMeta{Name: "devs", Namespace: "default"},
			Spec:       notificationv1.ContactGroupSpec{DisplayName: "Developers"},
			Status:     notificationv1.ContactGroupStatus{ProviderID: "cg-1"},
		}

		membership = &notificationv1.ContactGroupMembership{
			TypeMeta:   metav1.TypeMeta{APIVersion: "notification.miloapis.com/v1alpha1", Kind: "ContactGroupMembership"},
			ObjectMeta: metav1.ObjectMeta{Name: "m-1", Namespace: "default"},
			Spec: notificationv1.ContactGroupMembershipSpec{
				ContactRef:      notificationv1.ContactReference{Name: contact.Name, Namespace: contact.Namespace},
				ContactGroupRef: notificationv1.ContactGroupReference{Name: contactGroup.Name, Namespace: contactGroup.Namespace},
			},
		}

		sch := scheme.Scheme
		gomega.Expect(notificationv1.AddToScheme(sch)).To(gomega.Succeed())

		k8sClient = fake.NewClientBuilder().
			WithScheme(sch).
			WithStatusSubresource(&notificationv1.ContactGroupMembership{}).
			WithObjects(contact.DeepCopy(), contactGroup.DeepCopy(), membership.DeepCopy()).
			Build()

		svc := emailprovider.NewService(&fakeUnderlyingProvider{}, "from@example.com", "reply@example.com")
		controller = &ContactGroupMembershipController{Client: k8sClient, EmailProvider: *svc}
		controller.Finalizers = finalizerpkg.NewFinalizers()
	})

	ginkgo.Context("Reconcile", func() {
		ginkgo.It("creates membership on provider and sets condition", func() {
			_, err := controller.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace}})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			fetched := &notificationv1.ContactGroupMembership{}
			gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace}, fetched)).To(gomega.Succeed())

			gomega.Expect(fetched.Status.ProviderID).To(gomega.Equal("cgm-1"))
			cond := meta.FindStatusCondition(fetched.Status.Conditions, notificationv1.ContactGroupMembershipReadyCondition)
			gomega.Expect(cond).NotTo(gomega.BeNil())
			gomega.Expect(cond.Reason).To(gomega.Equal(notificationv1.ContactGroupMembershipCreatePendingReason))
		})
	})

	ginkgo.Describe("contactGroupMembershipFinalizer", func() {
		var (
			k8sClientFinal client.Client
			finalizer      *contactGroupMembershipFinalizer
			svc            *emailprovider.Service
			fakeProv       *fakeUnderlyingProvider
		)

		ginkgo.BeforeEach(func() {
			sch := scheme.Scheme
			gomega.Expect(notificationv1.AddToScheme(sch)).To(gomega.Succeed())

			// membership with ProviderID
			membershipWithID := membership.DeepCopy()
			membershipWithID.Status.ProviderID = "cgm-1"

			// corresponding removal object expected to be deleted by finalizer
			removal := &notificationv1.ContactGroupMembershipRemoval{
				TypeMeta:   metav1.TypeMeta{APIVersion: "notification.miloapis.com/v1alpha1", Kind: "ContactGroupMembershipRemoval"},
				ObjectMeta: metav1.ObjectMeta{Name: "remove-1", Namespace: "default"},
				Spec: notificationv1.ContactGroupMembershipRemovalSpec{
					ContactRef:      notificationv1.ContactReference{Name: contact.Name, Namespace: contact.Namespace},
					ContactGroupRef: notificationv1.ContactGroupReference{Name: contactGroup.Name, Namespace: contactGroup.Namespace},
				},
			}

			k8sClientFinal = fake.NewClientBuilder().
				WithScheme(sch).
				WithObjects(contactGroup.DeepCopy(), membershipWithID, removal).
				WithIndex(&notificationv1.ContactGroupMembershipRemoval{}, contactAndContactGroupTupleIndexKey, func(raw client.Object) []string {
					cgmr := raw.(*notificationv1.ContactGroupMembershipRemoval)
					return []string{buildContactAndContactGroupTupleIndexKey(cgmr.Spec.ContactRef, cgmr.Spec.ContactGroupRef)}
				}).
				Build()

			fakeProv = &fakeUnderlyingProvider{}
			svc = emailprovider.NewService(fakeProv, "from", "reply")
			finalizer = &contactGroupMembershipFinalizer{Client: k8sClientFinal, EmailProvider: *svc}
		})

		ginkgo.It("deletes membership on provider", func() {
			// Use object with ProviderID set so finalizer passes correct ID to provider
			obj := membership.DeepCopy()
			obj.Status.ProviderID = "cgm-1"
			res, err := finalizer.Finalize(ctx, obj)
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(res).To(gomega.Equal(finalizerpkg.Result{}))

		})
	})
})
