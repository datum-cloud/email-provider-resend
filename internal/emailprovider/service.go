package emailprovider

import (
	"context"
	"fmt"

	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"

	emailtemplating "go.miloapis.com/email-provider-resend/internal/emailtemplanting"
)

// Service ties rendering logic with an underlying EmailProvider.
type Service struct {
	provider EmailProvider // Actual email provider
	from     string        // Default From address
	replyTo  string        // Default Reply-To address
}

// NewService creates a new Service.
func NewService(provider EmailProvider, from, replyTo string) *Service {
	return &Service{
		provider: provider,
		from:     from,
		replyTo:  replyTo,
	}
}

// Send takes the CRD objects coming from Milo, renders the templates and finally
// hands the result to the configured Provider.
func (s *Service) Send(ctx context.Context,
	email *notificationmiloapiscomv1alpha1.Email,
	template *notificationmiloapiscomv1alpha1.EmailTemplate,
	recipientEmailAddress string,
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

	return s.provider.SendEmail(ctx, SendEmailInput{
		From:           s.from,
		ReplyTo:        s.replyTo,
		To:             []string{recipientEmailAddress},
		Cc:             email.Spec.CC,
		Bcc:            email.Spec.BCC,
		Subject:        subject,
		HtmlBody:       htmlBody,
		TextBody:       textBody,
		IdempotencyKey: string(email.UID),
	})
}
