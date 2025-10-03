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
	finalizerpkg "sigs.k8s.io/controller-runtime/pkg/finalizer"
)

var _ = ginkgo.Describe("ContactController", func() {
	var (
		ctx        context.Context
		k8sClient  client.Client
		controller *ContactController
		contact    *notificationv1.Contact
	)

	ginkgo.BeforeEach(func() {
		ctx = context.Background()

		// Prepare Contact object
		contact = &notificationv1.Contact{
			TypeMeta: metav1.TypeMeta{APIVersion: "notification.miloapis.com/v1alpha1", Kind: "Contact"},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "john-doe",
			},
			Spec: notificationv1.ContactSpec{
				FamilyName: "Doe",
				GivenName:  "John",
				Email:      "john@example.com",
			},
		}

		sch := scheme.Scheme
		gomega.Expect(notificationv1.AddToScheme(sch)).To(gomega.Succeed())

		indexFn := func(raw client.Object) []string {
			c := raw.(*notificationv1.ContactGroupMembership)
			return []string{buildContactNamespacedIndexKey(c.Spec.ContactRef.Name, c.Spec.ContactRef.Namespace)}
		}

		k8sClient = fake.NewClientBuilder().
			WithScheme(sch).
			WithStatusSubresource(&notificationv1.Contact{}).
			WithObjects(contact.DeepCopy()).
			WithIndex(&notificationv1.ContactGroupMembership{}, contactNamespacedIndexKey, indexFn).
			Build()

		controller = &ContactController{Client: k8sClient}
		controller.Finalizers = finalizerpkg.NewFinalizers()
	})

	ginkgo.Context("when the contact is created for the first time", func() {
		ginkgo.It("sets the Ready condition", func() {
			_, err := controller.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: contact.Name, Namespace: contact.Namespace}})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			fetched := &notificationv1.Contact{}
			gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: contact.Name, Namespace: contact.Namespace}, fetched)).To(gomega.Succeed())

			cond := meta.FindStatusCondition(fetched.Status.Conditions, notificationv1.ContactReadyCondition)
			gomega.Expect(cond).NotTo(gomega.BeNil())
			gomega.Expect(cond.Reason).To(gomega.Equal(notificationv1.ContactCreatedReason))
			gomega.Expect(cond.Status).To(gomega.Equal(metav1.ConditionTrue))
		})
	})

	ginkgo.Context("when the contact is updated", func() {
		ginkgo.It("sets the Updated condition", func() {
			// First reconcile to add Ready condition with observedGeneration 1
			_, err := controller.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: contact.Name, Namespace: contact.Namespace}})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Simulate spec update -> increase generation
			fetched := &notificationv1.Contact{}
			gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: contact.Name, Namespace: contact.Namespace}, fetched)).To(gomega.Succeed())
			fetched.Spec.FamilyName = "Smith"
			gomega.Expect(k8sClient.Update(ctx, fetched)).To(gomega.Succeed())

			// Kubernetes automatically bumps metadata.generation, but fake client will not.
			// We'll emulate by manually setting Generation.
			fetched.Generation = 2
			gomega.Expect(k8sClient.Update(ctx, fetched)).To(gomega.Succeed())

			// Reconcile again
			_, err = controller.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: contact.Name, Namespace: contact.Namespace}})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: contact.Name, Namespace: contact.Namespace}, fetched)).To(gomega.Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, notificationv1.ContactUpdatedCondition)
			gomega.Expect(cond).NotTo(gomega.BeNil())
			gomega.Expect(cond.ObservedGeneration).To(gomega.Equal(int64(2)))
		})
	})
})

var _ = ginkgo.Describe("contactFinalizer", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		finalizer *contactFinalizer
		contact   *notificationv1.Contact
		cgm       *notificationv1.ContactGroupMembership
	)

	ginkgo.BeforeEach(func() {
		ctx = context.Background()
		sch := scheme.Scheme
		gomega.Expect(notificationv1.AddToScheme(sch)).To(gomega.Succeed())

		contact = &notificationv1.Contact{
			TypeMeta:   metav1.TypeMeta{APIVersion: "notification.miloapis.com/v1alpha1", Kind: "Contact"},
			ObjectMeta: metav1.ObjectMeta{Name: "john-doe", Namespace: "default"},
			Spec:       notificationv1.ContactSpec{FamilyName: "Doe", GivenName: "John", Email: "john@example.com"},
		}

		cgm = &notificationv1.ContactGroupMembership{
			TypeMeta:   metav1.TypeMeta{APIVersion: "notification.miloapis.com/v1alpha1", Kind: "ContactGroupMembership"},
			ObjectMeta: metav1.ObjectMeta{Name: "member-1", Namespace: "default"},
			Spec: notificationv1.ContactGroupMembershipSpec{
				ContactRef:      notificationv1.ContactReference{Name: contact.Name, Namespace: contact.Namespace},
				ContactGroupRef: notificationv1.ContactGroupReference{Name: "group-1", Namespace: "default"},
			},
		}

		k8sClient = fake.NewClientBuilder().
			WithScheme(sch).
			WithObjects(contact.DeepCopy(), cgm.DeepCopy()).
			WithIndex(&notificationv1.ContactGroupMembership{}, contactNamespacedIndexKey, func(raw client.Object) []string {
				c := raw.(*notificationv1.ContactGroupMembership)
				return []string{buildContactNamespacedIndexKey(c.Spec.ContactRef.Name, c.Spec.ContactRef.Namespace)}
			}).
			WithIndex(&notificationv1.ContactGroupMembershipRemoval{}, contactNamespacedIndexKey, func(raw client.Object) []string {
				c := raw.(*notificationv1.ContactGroupMembershipRemoval)
				return []string{buildContactNamespacedIndexKey(c.Spec.ContactRef.Name, c.Spec.ContactRef.Namespace)}
			}).
			Build()
		finalizer = &contactFinalizer{Client: k8sClient}
	})

	ginkgo.It("deletes associated membership in same namespace", func() {
		res, err := finalizer.Finalize(ctx, contact.DeepCopy())
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(res).To(gomega.Equal(finalizerpkg.Result{}))

		list := &notificationv1.ContactGroupMembershipList{}
		gomega.Expect(k8sClient.List(ctx, list)).To(gomega.Succeed())
		gomega.Expect(list.Items).To(gomega.BeEmpty())
	})

	ginkgo.It("ignores memberships in other namespaces", func() {
		// recreate client with membership in another ns
		other := cgm.DeepCopy()
		other.Spec.ContactRef.Namespace = "other-ns"

		sch := scheme.Scheme
		k8sClient = fake.NewClientBuilder().
			WithScheme(sch).
			WithObjects(contact.DeepCopy(), other).
			WithIndex(&notificationv1.ContactGroupMembership{}, contactNamespacedIndexKey, func(raw client.Object) []string {
				c := raw.(*notificationv1.ContactGroupMembership)
				return []string{buildContactNamespacedIndexKey(c.Spec.ContactRef.Name, c.Spec.ContactRef.Namespace)}
			}).
			WithIndex(&notificationv1.ContactGroupMembershipRemoval{}, contactNamespacedIndexKey, func(raw client.Object) []string {
				c := raw.(*notificationv1.ContactGroupMembershipRemoval)
				return []string{buildContactNamespacedIndexKey(c.Spec.ContactRef.Name, c.Spec.ContactRef.Namespace)}
			}).
			Build()
		finalizer = &contactFinalizer{Client: k8sClient}

		_, err := finalizer.Finalize(ctx, contact.DeepCopy())
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		list := &notificationv1.ContactGroupMembershipList{}
		gomega.Expect(k8sClient.List(ctx, list)).To(gomega.Succeed())
		gomega.Expect(list.Items).To(gomega.HaveLen(1)) // still present
	})
})
