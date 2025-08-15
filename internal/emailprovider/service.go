package emailprovider

import (
	"context"
	"fmt"

	iammiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/iam/v1alpha1"
	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"

	emailtemplating "go.miloapis.com/email-provider-resend/internal/emailtemplanting"
)

// Service ties rendering logic with an underlying EmailProvider.
type Service struct {
	Provider EmailProvider // Actual email provider
	From     string        // Default From address
	ReplyTo  string        // Default Reply-To address
}

// Send takes the CRD objects coming from Milo, renders the templates and finally
// hands the result to the configured Provider.
func (s *Service) Send(ctx context.Context,
	email *notificationmiloapiscomv1alpha1.Email,
	template *notificationmiloapiscomv1alpha1.EmailTemplate,
	userRecipient *iammiloapiscomv1alpha1.User,
) (SendEmailOutput, error) {
	// variables are already validated by Milo webhooks
	// to match the referenced template
	vars := email.Spec.Variables

	output := SendEmailOutput{
		DeliveryID: "",
	}

	htmlBody, err := emailtemplating.RenderHTMLBodyTemplate(vars, template)
	if err != nil {
		return output, fmt.Errorf("render HTML body: %w", err)
	}

	textBody, err := emailtemplating.RenderTextBodyTemplate(vars, template)
	if err != nil {
		return output, fmt.Errorf("render text body: %w", err)
	}

	subject, err := emailtemplating.RenderSubjectTemplate(vars, template)
	if err != nil {
		return output, fmt.Errorf("render subject: %w", err)
	}

	return s.Provider.SendEmail(ctx, SendEmailInput{
		From:           s.From,
		ReplyTo:        s.ReplyTo,
		To:             []string{userRecipient.Spec.Email},
		Cc:             email.Spec.CC,
		Bcc:            email.Spec.BCC,
		Subject:        subject,
		HtmlBody:       htmlBody,
		TextBody:       textBody,
		IdempotencyKey: string(email.UID),
	})
}
