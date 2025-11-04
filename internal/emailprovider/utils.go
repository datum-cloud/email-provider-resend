package emailprovider

import (
	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"
)

// getDeterministicContactGroupName returns a deterministic contact group displayname for the contact group.
// As the email provider does not support namespaces or custom identifiers, we need to use a deterministic name for the contact group
// stored in the email provider.
func GetDeterministicContactGroupDisplayName(contactGroup *notificationmiloapiscomv1alpha1.ContactGroup) string {
	return string(contactGroup.UID)
}
