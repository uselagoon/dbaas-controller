package controller

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// errTypeToEventReason converts an error type to an event reason
// the error type is a string with potential dashes like "ingress-reconcile"
// the event reason is a string with potential camel case like "IngressReconcile"
func errTypeToEventReason(errType string) string {
	caser := cases.Title(language.English)
	parts := strings.Split(errType, "-")
	// capitalize the first letter of each part
	for i, part := range parts {
		parts[i] = caser.String(part)
	}
	return strings.Join(parts, "")
}
