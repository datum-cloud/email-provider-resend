package emailprovider

import (
	"context"
	"fmt"

	emailtemplating "go.miloapis.com/email-provider-resend/internal/emailtemplanting"
	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

// CreateContactGroup creates a contact group on the email provider.
func (s *Service) CreateContactGroup(ctx context.Context, cg notificationmiloapiscomv1alpha1.ContactGroup) (CreateContactGroupOutput, error) {
	displayName := GetDeterministicContactGroupDisplayName(&cg)
	return s.provider.CreateContactGroup(ctx, CreateContactGroupInput{
		DisplayName: displayName,
	})

}

// GetContactGroup returns the email provider contact group id of the contact group.
func (s *Service) GetContactGroup(ctx context.Context, cg notificationmiloapiscomv1alpha1.ContactGroup) (GetContactGroupOutput, error) {
	return s.provider.GetContactGroup(ctx, GetContactGroupInput{
		ContactGroupID: cg.Status.ProviderID,
	})
}

// DeleteContactGroup deletes a contact group on the email provider.
func (s *Service) DeleteContactGroup(ctx context.Context, cg notificationmiloapiscomv1alpha1.ContactGroup) (DeleteContactGroupOutput, error) {
	return s.provider.DeleteContactGroup(ctx, DeleteContactGroupInput{
		ContactGroupID: cg.Status.ProviderID,
	})
}

// GetContactGroupByDisplayName returns the email provider contact group by display name.
func (s *Service) GetContactGroupByDisplayName(ctx context.Context, displayName string) (GetContactGroupOutput, error) {
	contactGroups, err := s.provider.ListContactGroups(ctx)
	if err != nil {
		return GetContactGroupOutput{}, err
	}
	for _, contactGroup := range contactGroups.ContactGroups {
		if contactGroup.DisplayName == displayName {
			return contactGroup, nil
		}
	}
	return GetContactGroupOutput{}, errors.NewNotFound(schema.GroupResource{Group: "resend", Resource: "audiences"}, displayName)
}

// CreateContactGroupMembership creates a contact group membership on the email provider.
func (s *Service) CreateContactGroupMembershipIdempotent(ctx context.Context, contactGroup notificationmiloapiscomv1alpha1.ContactGroup, contact notificationmiloapiscomv1alpha1.Contact) (CreateContactGroupMembershipOutput, error) {
	existing, err := s.GetContactGroupMembershipByEmail(ctx, contactGroup, contact)
	if err != nil {
		if errors.IsNotFound(err) {
			return s.CreateContactGroupMembership(ctx, contactGroup, contact)
		}
		return CreateContactGroupMembershipOutput{}, err
	}

	return CreateContactGroupMembershipOutput(existing), nil
}

// GetContactGroupMembershipByEmail returns the contact group membership by email.
func (s *Service) GetContactGroupMembershipByEmail(ctx context.Context, contactGroup notificationmiloapiscomv1alpha1.ContactGroup, contact notificationmiloapiscomv1alpha1.Contact) (GetContactGroupMembershipByEmailOutput, error) {
	return s.provider.GetContactGroupMembershipByEmail(ctx, GetContactGroupMembershipByEmailInput{
		ContactGroupID: contactGroup.Status.ProviderID,
		Email:          contact.Spec.Email,
	})
}

// DeleteContactGroupMembership deletes a contact group membership on the email provider.
// This is an idempotent operation.
func (s *Service) DeleteContactGroupMembershipIdempotent(ctx context.Context, contactGroupMembership notificationmiloapiscomv1alpha1.ContactGroupMembership, contactGroup notificationmiloapiscomv1alpha1.ContactGroup) (DeleteContactGroupMembershipOutput, error) {
	return s.provider.DeleteContactGroupMembership(ctx, DeleteContactGroupMembershipInput{
		ContactGroupMembershipID: contactGroupMembership.Status.ProviderID,
		ContactGroupId:           contactGroup.Status.ProviderID,
	})
}

// CreateContactGroupMembership creates a contact group membership on the email provider.
func (s *Service) CreateContactGroupMembership(ctx context.Context, contactGroup notificationmiloapiscomv1alpha1.ContactGroup, contact notificationmiloapiscomv1alpha1.Contact) (CreateContactGroupMembershipOutput, error) {
	return s.provider.CreateContactGroupMembership(ctx, CreateContactGroupMembershipInput{
		ContactGroupID: contactGroup.Status.ProviderID,
		Email:          contact.Spec.Email,
		GivenName:      contact.Spec.GivenName,
		FamilyName:     contact.Spec.FamilyName,
	})
}

// UpdateContactGroupMembership updates a contact on the email provider.
// As Resend does not support updating the email address, we delete the existing contact group membership and create a new one.
// This would save us some API calls.
func (s *Service) UpdateContactGroupMembership(ctx context.Context,
	contactGroupMembership notificationmiloapiscomv1alpha1.ContactGroupMembership,
	contactGroup notificationmiloapiscomv1alpha1.ContactGroup,
	contact notificationmiloapiscomv1alpha1.Contact) (CreateContactGroupMembershipOutput, error) {
	deleted, err := s.DeleteContactGroupMembershipIdempotent(ctx, contactGroupMembership, contactGroup)
	if err != nil {
		return CreateContactGroupMembershipOutput{}, err
	}
	if !deleted.Deleted {
		return CreateContactGroupMembershipOutput{}, fmt.Errorf("failed to delete contact group membership during update")
	}
	return s.CreateContactGroupMembership(ctx, contactGroup, contact)
}
