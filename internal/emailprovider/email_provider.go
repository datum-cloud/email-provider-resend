package emailprovider

import "context"

// SendEmailInput contains all the data required to send an email regardless of the underlying provider.
type SendEmailInput struct {
	From           string
	ReplyTo        string
	IdempotencyKey string
	To             []string
	Cc             []string
	Bcc            []string
	Subject        string
	HtmlBody       string
	TextBody       string
}

// SendEmailOutput contains the output of the email provider
type SendEmailOutput struct {
	DeliveryID string
}

// EmailProvider defines the contract every e-mail provider (Resend, SES, Mailgun, …) must fulfil.
type EmailProvider interface {
	SendEmail(ctx context.Context, input SendEmailInput) (SendEmailOutput, error)
}
