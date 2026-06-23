//go:build !lite

package support

import "regexp"

// placeholderRe matches {{key}} patterns with optional whitespace inside braces.
var placeholderRe = regexp.MustCompile(`{{\s*(\w+)\s*}}`)

// SubstitutePlaceholders replaces {{key}} patterns in body with values from vars.
// If a key is not found in vars, the placeholder is left unchanged.
func SubstitutePlaceholders(body string, vars map[string]string) string {
	return placeholderRe.ReplaceAllStringFunc(body, func(match string) string {
		// Extract the key name from the match
		sub := placeholderRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		key := sub[1]
		if val, ok := vars[key]; ok {
			return val
		}
		return match
	})
}

// DefaultPlaceholders returns a list of commonly supported placeholder names
// for canned responses.
func DefaultPlaceholders() []string {
	return []string{
		"customer_name",
		"plan_name",
		"expiry_date",
		"username",
		"node_name",
		"ticket_id",
	}
}
