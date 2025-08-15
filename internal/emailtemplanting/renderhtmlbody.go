package emailtemplating

import (
	"bytes"
	"fmt"
	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"
	htmltemplate "html/template"
)

// renderHTMLTemplate renders an HTML template with the given variables
func RenderHTMLBodyTemplate(variables []notificationmiloapiscomv1alpha1.EmailVariable, template *notificationmiloapiscomv1alpha1.EmailTemplate) (string, error) {
	htmlTemplate := template.Spec.HTMLBody
	if htmlTemplate == "" {
		return "", nil
	}

	tmpl, err := htmltemplate.New("html").Parse(htmlTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, convertVariables(variables)); err != nil {
		return "", fmt.Errorf("failed to execute HTML template: %w", err)
	}

	return buf.String(), nil
}
