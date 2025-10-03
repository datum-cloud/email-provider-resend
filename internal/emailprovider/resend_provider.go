package emailprovider

import (
	"context"
	"fmt"

	"github.com/resend/resend-go/v2"
	rtime "go.miloapis.com/email-provider-resend/internal/resend"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

// CreateContactGroup satisfies the EmailProvider interface. It returns the resend contact group id of the contact group.
func (r *ResendEmailProvider) CreateContactGroup(ctx context.Context, input CreateContactGroupInput) (CreateContactGroupOutput, error) {
	output := CreateContactGroupOutput{
		ContactGroupID: "",
	}

	resp, err := r.client.Audiences.Create(&resend.CreateAudienceRequest{
		Name: input.DisplayName,
	})
	if err != nil {
		return output, fmt.Errorf("failed to create contact group using resend: %w", err)
	}

	output.ContactGroupID = resp.Id

	return output, nil
}

// GetContactGroup satisfies the EmailProvider interface. It returns the resend contact group id of the contact group.
func (r *ResendEmailProvider) GetContactGroup(ctx context.Context, input GetContactGroupInput) (GetContactGroupOutput, error) {
	output := GetContactGroupOutput{}

	resp, err := r.client.Audiences.Get(input.ContactGroupID)
	if err != nil {
		return output, TranslateResendError(err, schema.GroupResource{Group: "resend", Resource: "audiences"}, input.ContactGroupID)
	}

	output.ContactGroupID = resp.Id
	output.DisplayName = resp.Name

	// Parse the timestamp returned by Resend using the resilient ResendTime helper.
	var rt rtime.ResendTime
	if err := rt.UnmarshalJSON([]byte(fmt.Sprintf("%q", resp.CreatedAt))); err != nil {
		return output, fmt.Errorf("failed to parse created at get contact group using resend: %w", err)
	}
	output.CreatedAt = rt.Time

	return output, nil
}

// DeleteContactGroup satisfies the EmailProvider interface. It returns the resend contact group id of the contact group.
func (r *ResendEmailProvider) DeleteContactGroup(ctx context.Context, input DeleteContactGroupInput) (DeleteContactGroupOutput, error) {
	output := DeleteContactGroupOutput{}

	resp, err := r.client.Audiences.Remove(input.ContactGroupID)
	if err != nil {
		return output, TranslateResendError(err, schema.GroupResource{Group: "resend", Resource: "audiences"}, input.ContactGroupID)
	}

	output.Deleted = resp.Deleted
	output.ContactGroupID = input.ContactGroupID

	return output, nil
}

// ListContactGroups satisfies the EmailProvider interface. It returns the list of contact groups.
func (r *ResendEmailProvider) ListContactGroups(ctx context.Context) (ListContactGroupsOutput, error) {
	output := ListContactGroupsOutput{}

	resp, err := r.client.Audiences.List()
	if err != nil {
		return output, fmt.Errorf("failed to list contact groups using resend: %w", err)
	}

	for _, audience := range resp.Data {
		output.ContactGroups = append(output.ContactGroups, GetContactGroupOutput{
			ContactGroupID: audience.Id,
			DisplayName:    audience.Name,
		})
	}

	return output, nil
}

// CreateContactGroupMembership satisfies the EmailProvider interface. It returns the resend contact group membership id of the contact group membership.
func (r *ResendEmailProvider) CreateContactGroupMembership(ctx context.Context, input CreateContactGroupMembershipInput) (CreateContactGroupMembershipOutput, error) {
	output := CreateContactGroupMembershipOutput{
		ContactGroupMembershipID: "",
	}

	resp, err := r.client.Contacts.Create(&resend.CreateContactRequest{
		AudienceId:   input.ContactGroupID,
		Email:        input.Email,
		FirstName:    input.GivenName,
		LastName:     input.FamilyName,
		Unsubscribed: false,
	})
	if err != nil {
		return output, TranslateResendError(err, schema.GroupResource{Group: "resend", Resource: "contacts"}, input.ContactGroupID)
	}

	output.ContactGroupMembershipID = resp.Id

	return output, nil
}

// GetContactGroupMembershipByEmail satisfies the EmailProvider interface. It returns the resend contact group membership id of the contact group membership.
func (r *ResendEmailProvider) GetContactGroupMembershipByEmail(ctx context.Context, input GetContactGroupMembershipByEmailInput) (GetContactGroupMembershipByEmailOutput, error) {
	output := GetContactGroupMembershipByEmailOutput{
		ContactGroupMembershipID: "",
	}

	resp, err := r.client.Contacts.Get(input.ContactGroupID, input.Email)
	if err != nil {
		return output, TranslateResendError(err, schema.GroupResource{Group: "resend", Resource: "contacts"}, input.Email)
	}

	output.ContactGroupMembershipID = resp.Id

	return output, nil
}

// DeleteContactGroupMembership satisfies the EmailProvider interface. It returns the resend contact group membership id of the contact group membership.
// This is an idempotent operation.
func (r *ResendEmailProvider) DeleteContactGroupMembership(ctx context.Context, input DeleteContactGroupMembershipInput) (DeleteContactGroupMembershipOutput, error) {
	output := DeleteContactGroupMembershipOutput{
		ContactGroupMembershipID: "",
		Deleted:                  false,
	}

	resp, err := r.client.Contacts.Remove(input.ContactGroupId, input.ContactGroupMembershipID)
	if err != nil {
		return output, TranslateResendError(err, schema.GroupResource{Group: "resend", Resource: "contacts"}, input.ContactGroupId)
	}

	output.Deleted = resp.Deleted
	return output, nil
}
