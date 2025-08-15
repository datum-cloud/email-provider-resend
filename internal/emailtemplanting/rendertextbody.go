package emailtemplating

import (
	"bytes"
	"fmt"
	texttemplate "text/template"

	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"
)

// renderTextBodyTemplate renders a text body template with the given variables
func RenderTextBodyTemplate(variables []notificationmiloapiscomv1alpha1.EmailVariable, template *notificationmiloapiscomv1alpha1.EmailTemplate) (string, error) {
	return renderTextTemplate(variables, template.Spec.TextBody)
}

// renderSubjectTemplate renders a subject template with the given variables
func RenderSubjectTemplate(variables []notificationmiloapiscomv1alpha1.EmailVariable, template *notificationmiloapiscomv1alpha1.EmailTemplate) (string, error) {
	return renderTextTemplate(variables, template.Spec.Subject)
}

// renderTextTemplate renders a text template with the given variables
func renderTextTemplate(variables []notificationmiloapiscomv1alpha1.EmailVariable, textTemplate string) (string, error) {
	if textTemplate == "" {
		return "", nil
	}

	tmpl, err := texttemplate.New("text").Parse(textTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse text template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, convertVariables(variables)); err != nil {
		return "", fmt.Errorf("failed to execute text template: %w", err)
	}

	return buf.String(), nil
}
