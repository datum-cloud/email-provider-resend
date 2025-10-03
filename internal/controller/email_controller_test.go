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
	"time"

	ginko "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
	iammiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/iam/v1alpha1"
	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"go.miloapis.com/email-provider-resend/internal/config"
	"go.miloapis.com/email-provider-resend/internal/emailprovider"
)

// fakeEmailProvider implements emailprovider.EmailProvider for testing purposes.
type fakeEmailProvider struct {
	callCount int
	output    emailprovider.SendEmailOutput
	err       error
}

func (f *fakeEmailProvider) SendEmail(ctx context.Context, input emailprovider.SendEmailInput) (emailprovider.SendEmailOutput, error) {
	f.callCount++
	return f.output, f.err
}

func (f *fakeEmailProvider) CreateContactGroup(ctx context.Context, input emailprovider.CreateContactGroupInput) (emailprovider.CreateContactGroupOutput, error) {
	return emailprovider.CreateContactGroupOutput{}, nil
}

func (f *fakeEmailProvider) GetContactGroup(ctx context.Context, input emailprovider.GetContactGroupInput) (emailprovider.GetContactGroupOutput, error) {
	return emailprovider.GetContactGroupOutput{}, nil
}

func (f *fakeEmailProvider) DeleteContactGroup(ctx context.Context, input emailprovider.DeleteContactGroupInput) (emailprovider.DeleteContactGroupOutput, error) {
	return emailprovider.DeleteContactGroupOutput{}, nil
}

func (f *fakeEmailProvider) ListContactGroups(ctx context.Context) (emailprovider.ListContactGroupsOutput, error) {
	return emailprovider.ListContactGroupsOutput{}, nil
}

func (f *fakeEmailProvider) CreateContactGroupMembership(ctx context.Context, input emailprovider.CreateContactGroupMembershipInput) (emailprovider.CreateContactGroupMembershipOutput, error) {
	return emailprovider.CreateContactGroupMembershipOutput{}, nil
}

func (f *fakeEmailProvider) GetContactGroupMembershipByEmail(ctx context.Context, input emailprovider.GetContactGroupMembershipByEmailInput) (emailprovider.GetContactGroupMembershipByEmailOutput, error) {
	return emailprovider.GetContactGroupMembershipByEmailOutput{}, nil
}

func (f *fakeEmailProvider) DeleteContactGroupMembership(ctx context.Context, input emailprovider.DeleteContactGroupMembershipInput) (emailprovider.DeleteContactGroupMembershipOutput, error) {
	return emailprovider.DeleteContactGroupMembershipOutput{}, nil
}

var _ = ginko.Describe("EmailController.Reconcile", func() {
	var (
		ctx        context.Context
		fakeProv   *fakeEmailProvider
		controller *EmailController
		emailObj   *notificationmiloapiscomv1alpha1.Email
		k8sClient  client.Client
	)

	ginko.BeforeEach(func() {
		ctx = context.Background()

		// Prepare a basic Email object
		emailObj = &notificationmiloapiscomv1alpha1.Email{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "notification.miloapis.com/v1alpha1",
				Kind:       "Email",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test-email",
				UID:       types.UID("uid-test"),
			},
			Spec: notificationmiloapiscomv1alpha1.EmailSpec{
				TemplateRef: notificationmiloapiscomv1alpha1.TemplateReference{
					Name: "welcome-template",
				},
				Recipient: notificationmiloapiscomv1alpha1.EmailRecipient{
					UserRef: notificationmiloapiscomv1alpha1.EmailUserReference{
						Name: "user-1",
					},
				},
				Priority: notificationmiloapiscomv1alpha1.EmailPriorityNormal,
			},
		}
		// Create dependent cluster-scoped objects
		template := &notificationmiloapiscomv1alpha1.EmailTemplate{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "notification.miloapis.com/v1alpha1",
				Kind:       "EmailTemplate",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "welcome-template",
			},
			Spec: notificationmiloapiscomv1alpha1.EmailTemplateSpec{
				Subject:  "Welcome",
				HTMLBody: "", // Intentionally empty to bypass template rendering logic
				TextBody: "", // Same as above
			},
		}

		user := &iammiloapiscomv1alpha1.User{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "iam.miloapis.com/v1alpha1",
				Kind:       "User",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "user-1",
			},
			Spec: iammiloapiscomv1alpha1.UserSpec{
				Email: "recipient@example.com",
			},
		}

		// Initialise the scheme used by the fake client
		sch := scheme.Scheme
		// Register CRD types into the scheme so the fake client recognises them
		gomega.Expect(iammiloapiscomv1alpha1.AddToScheme(sch)).To(gomega.Succeed())
		gomega.Expect(notificationmiloapiscomv1alpha1.AddToScheme(sch)).To(gomega.Succeed())

		k8sClient = fake.NewClientBuilder().
			WithScheme(sch).
			WithStatusSubresource(&notificationmiloapiscomv1alpha1.Email{}).
			WithObjects(emailObj.DeepCopy(), template, user).
			Build()

		// Setup the fake provider wrapped by the Service abstraction used by the controller
		fakeProv = &fakeEmailProvider{
			output: emailprovider.SendEmailOutput{DeliveryID: "delivery-123"},
		}
		service := emailprovider.NewService(fakeProv, "from@example.com", "reply@example.com")

		// Minimal retry config (1 s for every priority) to simplify assertions
		conf, err := config.NewEmailControllerConfig(time.Second, time.Second, time.Second)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		controller = &EmailController{
			Client:        k8sClient,
			EmailProvider: *service,
			Config:        *conf,
		}
	})

	ginko.Context("when the email has not been sent yet", func() {
		ginko.It("sends the email and updates the status", func() {
			res, err := controller.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: emailObj.Name, Namespace: emailObj.Namespace}})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(res).To(gomega.Equal(ctrl.Result{}))
			gomega.Expect(fakeProv.callCount).To(gomega.Equal(1)) // One call to the provider

			fetched := &notificationmiloapiscomv1alpha1.Email{}
			gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: emailObj.Name, Namespace: emailObj.Namespace}, fetched)).To(gomega.Succeed())
			gomega.Expect(fetched.Status.ProviderID).To(gomega.Equal("delivery-123"))
		})
	})

	ginko.Context("when the email was already sent", func() {
		ginko.It("does not send again", func() {
			// Mark the resource as already delivered
			existing := &notificationmiloapiscomv1alpha1.Email{}
			gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: emailObj.Name, Namespace: emailObj.Namespace}, existing)).To(gomega.Succeed())
			existing.Status.ProviderID = "already-delivered"
			gomega.Expect(k8sClient.Status().Update(ctx, existing)).To(gomega.Succeed())

			res, err := controller.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: emailObj.Name, Namespace: emailObj.Namespace}})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(res).To(gomega.Equal(ctrl.Result{}))
			gomega.Expect(fakeProv.callCount).To(gomega.Equal(0)) // No call to the provider
		})
	})

	ginko.Context("when the provider returns an error", func() {
		ginko.It("requeues the reconcile after the configured delay", func() {
			fakeProv.err = fmt.Errorf("provider failure")

			res, err := controller.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: emailObj.Name, Namespace: emailObj.Namespace}})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(res.RequeueAfter).To(gomega.Equal(time.Second))
			gomega.Expect(fakeProv.callCount).To(gomega.Equal(1))
		})
	})

	ginko.Context("when the recipient is specified by email address", func() {
		ginko.BeforeEach(func() {
			// Override the recipient to use EmailAddress instead of UserRef
			emailObj.Spec.Recipient = notificationmiloapiscomv1alpha1.EmailRecipient{
				EmailAddress: "recipient@example.com",
			}

			// Rebuild the fake client without needing a User object
			sch := scheme.Scheme
			gomega.Expect(iammiloapiscomv1alpha1.AddToScheme(sch)).To(gomega.Succeed())
			gomega.Expect(notificationmiloapiscomv1alpha1.AddToScheme(sch)).To(gomega.Succeed())

			k8sClient = fake.NewClientBuilder().
				WithScheme(sch).
				WithStatusSubresource(&notificationmiloapiscomv1alpha1.Email{}).
				WithObjects(emailObj.DeepCopy(), &notificationmiloapiscomv1alpha1.EmailTemplate{
					TypeMeta:   metav1.TypeMeta{APIVersion: "notification.miloapis.com/v1alpha1", Kind: "EmailTemplate"},
					ObjectMeta: metav1.ObjectMeta{Name: "welcome-template"},
					Spec:       notificationmiloapiscomv1alpha1.EmailTemplateSpec{Subject: "Welcome"},
				}).
				Build()

			// Reset the fake provider call count
			fakeProv.callCount = 0

			// Recreate the service and controller with the new client
			service := emailprovider.NewService(fakeProv, "from@example.com", "reply@example.com")
			conf, err := config.NewEmailControllerConfig(time.Second, time.Second, time.Second)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			controller = &EmailController{Client: k8sClient, EmailProvider: *service, Config: *conf}
		})

		ginko.It("sends the email and updates the status", func() {
			res, err := controller.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: emailObj.Name, Namespace: emailObj.Namespace}})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(res).To(gomega.Equal(ctrl.Result{}))
			gomega.Expect(fakeProv.callCount).To(gomega.Equal(1))

			fetched := &notificationmiloapiscomv1alpha1.Email{}
			gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: emailObj.Name, Namespace: emailObj.Namespace}, fetched)).To(gomega.Succeed())
			gomega.Expect(fetched.Status.ProviderID).To(gomega.Equal("delivery-123"))
		})
	})
})
