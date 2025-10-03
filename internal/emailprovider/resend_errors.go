package emailprovider

import (
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TranslateResendError inspects the error returned by the Resend SDK.
// If it corresponds to a "not found" situation it converts it into a
// Kubernetes NotFound error (so callers can rely on apierrors.IsNotFound).
//
// The caller must specify which GroupResource and name were being requested
// so the generated error contains correct metadata.
func TranslateResendError(err error, gr schema.GroupResource, name string) error {
	if err == nil {
		return nil
	}

	// The Resend SDK returns plain errors, typically in the form:
	//   "[ERROR]: <Resource> not found"
	// We'll match the generic "not found" suffix.
	if strings.Contains(strings.ToLower(err.Error()), "not found") {
		return apierrors.NewNotFound(gr, name)
	}

	return err
}
