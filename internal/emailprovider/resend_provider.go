package emailprovider

import (
	"context"
	"fmt"

	"github.com/resend/resend-go/v2"
)

// ResendEmailProvider is an implementation of EmailProvider that delivers e-mails
// using Resend (https://resend.com/).
type ResendEmailProvider struct {
	client *resend.Client
}

// NewResendEmailProviderFromAPIKey instantiates the provider from the given
// API key.
func NewResendEmailProvider(apiKey string) *ResendEmailProvider {
	return &ResendEmailProvider{client: resend.NewClient(apiKey)}
}

// SendEmail satisfies the EmailProvider interface. It returns the resend delivery id of the email.
func (r *ResendEmailProvider) SendEmail(ctx context.Context, input SendEmailInput) (SendEmailOutput, error) {
	output := SendEmailOutput{
		DeliveryID: "",
	}

	resp, err := r.client.Emails.Send(&resend.SendEmailRequest{
		From:    input.From,
		ReplyTo: input.ReplyTo,
		To:      input.To,
		Cc:      input.Cc,
		Bcc:     input.Bcc,
		Subject: input.Subject,
		Html:    input.HtmlBody,
		Text:    input.TextBody,
		Headers: map[string]string{
			"IdempotencyKey": input.IdempotencyKey,
		},
	})
	if err != nil {
		return output, fmt.Errorf("failed to send email using resend: %w", err)
	}

	output.DeliveryID = resp.Id

	return output, nil
}
