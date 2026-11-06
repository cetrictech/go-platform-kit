package redact

import (
	"net/url"
	"regexp"
	"strings"
)

var credentialAssignments = regexp.MustCompile(`(?i)(password|passwd|authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|token|client[_-]?secret|secret)\s*[:=]\s*(?:"[^"]*"|'[^']*'|(?:(?:bearer|basic)\s+)?[^\s,;]+)`)
var diagnosticURLs = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s"'<>]+`)
var privatePEM = regexp.MustCompile(`(?s)-----BEGIN (?:RSA |EC |ENCRYPTED |OPENSSH )?PRIVATE KEY-----.*?(?:-----END [^-]*PRIVATE KEY-----|$)`)

// SafeText masks common credential assignments, private-key PEM blocks and credential-bearing URLs.
// Application-specific unlabeled secrets still require an explicit application sanitizer and must not be logged.
func SafeText(s string) string {
	if len(s) > 16<<10 {
		s = s[:16<<10] + "[TRUNCATED]"
	}
	if strings.Contains(s, "PRIVATE KEY-----") {
		s = privatePEM.ReplaceAllString(s, "[REDACTED_PRIVATE_KEY]")
	}
	if strings.Contains(s, "://") {
		s = diagnosticURLs.ReplaceAllStringFunc(s, func(raw string) string {
			u, e := url.Parse(raw)
			if e != nil {
				return "[INVALID_URL]"
			}
			return SafeURL(*u)
		})
	}
	if strings.ContainsAny(s, ":=") {
		s = credentialAssignments.ReplaceAllString(s, "$1=[REDACTED]")
	}
	return s
}
