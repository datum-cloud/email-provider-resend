package emailtemplating

import (
	notificationmiloapiscomv1alpha1 "go.miloapis.com/milo/pkg/apis/notification/v1alpha1"
)

// convertVariables transforms a slice of EmailVariable into a map[string]string
// so templates can reference variables directly via {{ .MyVar }} rather than
// iterating over a slice. If duplicate names exist, the last one wins.
func convertVariables(vars []notificationmiloapiscomv1alpha1.EmailVariable) map[string]string {
	m := make(map[string]string, len(vars))
	for _, v := range vars {
		m[v.Name] = v.Value
	}
	return m
}
