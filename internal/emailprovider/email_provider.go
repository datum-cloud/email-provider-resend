package emailprovider

import (
	"context"
	"time"
)

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

// CreateContactGroupInput contains the input of the email provider
type CreateContactGroupInput struct {
	DisplayName string
}

// CreateContactGroupOutput contains the output of the email provider
type CreateContactGroupOutput struct {
	ContactGroupID string
}

// GetContactGroupInput contains the input of the email provider
type GetContactGroupInput struct {
	ContactGroupID string
}

// GetContactGroupOutput contains the output of the email provider
type GetContactGroupOutput struct {
	ContactGroupID string
	DisplayName    string
	CreatedAt      time.Time
}

// DeleteContactGroupInput contains the input of the email provider
type DeleteContactGroupInput struct {
	ContactGroupID string
}

// DeleteContactGroupOutput contains the output of the email provider
type DeleteContactGroupOutput struct {
	ContactGroupID string
	Deleted        bool
}

// ListContactGroupsOutput contains the output of the email provider
type ListContactGroupsOutput struct {
	ContactGroups []GetContactGroupOutput
}

// CreateContactGroupMembershipInput contains the input of the email provider
type CreateContactGroupMembershipInput struct {
	ContactGroupID string
	Email          string
	GivenName      string
	FamilyName     string
}

// CreateContactGroupMembershipOutput contains the output of the email provider
type CreateContactGroupMembershipOutput struct {
	ContactGroupMembershipID string
}

// GetContactGroupMembershipByEmailInput contains the input of the email provider
type GetContactGroupMembershipByEmailInput struct {
	ContactGroupID string
	Email          string
}

// GetContactGroupMembershipByEmailOutput contains the output of the email provider
type GetContactGroupMembershipByEmailOutput struct {
	ContactGroupMembershipID string
}

// DeleteContactGroupMembershipInput contains the input of the email provider
type DeleteContactGroupMembershipInput struct {
	ContactGroupMembershipID string
	ContactGroupId           string
}

// DeleteContactGroupMembershipOutput contains the output of the email provider
type DeleteContactGroupMembershipOutput struct {
	ContactGroupMembershipID string
	Deleted                  bool
}

// EmailProvider defines the contract every e-mail provider (Resend, SES, Mailgun, …) must fulfil.
type EmailProvider interface {
	SendEmail(ctx context.Context, input SendEmailInput) (SendEmailOutput, error)
	CreateContactGroup(ctx context.Context, input CreateContactGroupInput) (CreateContactGroupOutput, error)
	GetContactGroup(ctx context.Context, input GetContactGroupInput) (GetContactGroupOutput, error)
	DeleteContactGroup(ctx context.Context, input DeleteContactGroupInput) (DeleteContactGroupOutput, error)
	ListContactGroups(ctx context.Context) (ListContactGroupsOutput, error)
	CreateContactGroupMembership(ctx context.Context, input CreateContactGroupMembershipInput) (CreateContactGroupMembershipOutput, error)
	GetContactGroupMembershipByEmail(ctx context.Context, input GetContactGroupMembershipByEmailInput) (GetContactGroupMembershipByEmailOutput, error)
	DeleteContactGroupMembership(ctx context.Context, input DeleteContactGroupMembershipInput) (DeleteContactGroupMembershipOutput, error)
}
