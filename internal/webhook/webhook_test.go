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

// providerIDIndex extracts the Email.Status.ProviderID for field indexing.
func providerIDIndex(o crtclient.Object) []string {
	email := o.(*notificationv1alpha1.Email)
	if email.Status.ProviderID == "" {
		return nil
	}
	return []string{email.Status.ProviderID}
}

func buildScheme(t *testing.T) *runtime.Scheme {
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

func TestNewResendWebhookV1_Endpoint(t *testing.T) {
	wh := NewResendWebhookV1(nil)
	expected := "/apis/emailnotification.k8s.io/v1/resend/emails"
	if wh.Endpoint != expected {
		t.Fatalf("unexpected endpoint: got %s want %s", wh.Endpoint, expected)
	}
}

func TestNewResendWebhookV1_EmailNotFound(t *testing.T) {
	scheme := buildScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&notificationv1alpha1.Email{}, "status.providerID", providerIDIndex).
		Build()

	wh := NewResendWebhookV1(k8sClient)

	evt := resend.ParsedEvent{
		Envelope: resend.EventEnvelope{Type: resend.EventTypeSent},
		Base:     resend.EmailBase{EmailID: "provider-1"},
	}

	resp := wh.Handler.Handle(context.TODO(), Request{Event: evt})
	if resp.HttpStatus != http.StatusNotFound {
		t.Fatalf("expected %d got %d", http.StatusNotFound, resp.HttpStatus)
	}
}

func TestNewResendWebhookV1_SuccessPath(t *testing.T) {
	scheme := buildScheme(t)

	email := &notificationv1alpha1.Email{
		TypeMeta: metav1.TypeMeta{APIVersion: notificationv1alpha1.SchemeGroupVersion.String(), Kind: "Email"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-email",
			Namespace: "default",
		},
		Status: notificationv1alpha1.EmailStatus{
			ProviderID: "provider-xyz",
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&notificationv1alpha1.Email{}).
		WithIndex(&notificationv1alpha1.Email{}, "status.providerID", providerIDIndex).
		WithObjects(email).Build()

	wh := NewResendWebhookV1(k8sClient)

	evt := resend.ParsedEvent{
		Envelope: resend.EventEnvelope{Type: resend.EventTypeDelivered},
		Base:     resend.EmailBase{EmailID: "provider-xyz"},
	}

	resp := wh.Handler.Handle(context.TODO(), Request{Event: evt})
	if resp.HttpStatus != http.StatusOK {
		t.Fatalf("expected %d got %d", http.StatusOK, resp.HttpStatus)
	}

	// Verify email status was updated
	updated := &notificationv1alpha1.Email{}
	if err := k8sClient.Get(context.TODO(), crtclient.ObjectKey{Namespace: "default", Name: "test-email"}, updated); err != nil {
		t.Fatalf("failed to fetch updated email: %v", err)
	}
	// There should be at least one condition with status=True and reason=Delivered
	found := false
	for _, c := range updated.Status.Conditions {
		if c.Type == notificationv1alpha1.EmailDeliveredCondition && c.Status == metav1.ConditionTrue {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("delivered condition not found or not true: %+v", updated.Status.Conditions)
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
