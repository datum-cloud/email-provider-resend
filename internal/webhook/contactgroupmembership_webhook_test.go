package webhook

import (
	"context"
	"net/http"
	"testing"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"go.miloapis.com/email-provider-resend/internal/resend"
	notificationv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"
	crtclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// contactProviderIDIndex extracts the Contact.Status.ProviderID for field indexing.
func contactProviderIDIndex(o crtclient.Object) []string {
	contact := o.(*notificationv1alpha1.Contact)
	if contact.Status.ProviderID == "" {
		return nil
	}
	return []string{contact.Status.ProviderID}
}

func buildContactScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := notificationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add notification scheme: %v", err)
	}
	if err := eventsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add events scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	return scheme
}

func TestNewResendContactWebhookV1_Endpoint(t *testing.T) {
	wh := NewResendContactWebhookV1(nil)
	expected := "/apis/emailnotification.k8s.io/v1/resend/contactgroupmemberships"
	if wh.Endpoint != expected {
		t.Fatalf("unexpected endpoint: got %s want %s", wh.Endpoint, expected)
	}
}

func TestNewResendContactWebhookV1_CGMNotFound(t *testing.T) {
	scheme := buildContactScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&notificationv1alpha1.Contact{}, contactStatusProviderIDIndexKey, contactProviderIDIndex).
		Build()

	wh := NewResendContactWebhookV1(k8sClient)

	evt := resend.ParsedContactEvent{
		Envelope: resend.ContactEventEnvelope{Type: resend.ContactCreated},
		Contact:  resend.ContactBase{ID: "provider-missing"},
	}

	resp := wh.Handler.Handle(context.TODO(), Request{ContactEvent: &evt})
	if resp.HttpStatus != http.StatusNotFound {
		t.Fatalf("expected %d got %d", http.StatusNotFound, resp.HttpStatus)
	}
}

func TestNewResendContactWebhookV1_SuccessPath(t *testing.T) {
	scheme := buildContactScheme(t)

	contact := &notificationv1alpha1.Contact{
		TypeMeta: metav1.TypeMeta{APIVersion: notificationv1alpha1.SchemeGroupVersion.String(), Kind: "Contact"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-contact",
			Namespace: "default",
		},
		Status: notificationv1alpha1.ContactStatus{
			ProviderID: "provider-xyz",
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&notificationv1alpha1.Contact{}).
		WithIndex(&notificationv1alpha1.Contact{}, contactStatusProviderIDIndexKey, contactProviderIDIndex).
		WithObjects(contact).Build()

	wh := NewResendContactWebhookV1(k8sClient)

	evt := resend.ParsedContactEvent{
		Envelope: resend.ContactEventEnvelope{Type: resend.ContactCreated},
		Contact:  resend.ContactBase{ID: "provider-xyz"},
	}

	resp := wh.Handler.Handle(context.TODO(), Request{ContactEvent: &evt})
	if resp.HttpStatus != http.StatusOK {
		t.Fatalf("expected %d got %d", http.StatusOK, resp.HttpStatus)
	}

	// Verify CGM status was updated
	updated := &notificationv1alpha1.Contact{}
	if err := k8sClient.Get(context.TODO(), crtclient.ObjectKey{Namespace: "default", Name: "test-contact"}, updated); err != nil {
		t.Fatalf("failed to fetch updated contact: %v", err)
	}
	if len(updated.Status.Conditions) == 0 {
		t.Fatalf("expected at least 1 condition, got 0")
	}
	found := false
	for _, c := range updated.Status.Conditions {
		if c.Type == notificationv1alpha1.ContactReadyCondition && c.Status == metav1.ConditionTrue {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ready condition not found or not true: %+v", updated.Status.Conditions)
	}

	// Verify a Kubernetes Event was created
	evList := &eventsv1.EventList{}
	if err := k8sClient.List(context.TODO(), evList); err != nil {
		t.Fatalf("failed to list events: %v", err)
	}
	if len(evList.Items) == 0 {
		t.Fatalf("expected at least 1 event recorded, got 0")
	}
}
