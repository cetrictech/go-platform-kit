// Package redact shares structured secret handling without cryptographic dependencies.
package redact

import "strings"

// Sensitive identifies structured fields that must never be logged.
func Sensitive(key string) bool {
	k := key
	if strings.ContainsAny(k, "_- ") {
		k = strings.Map(func(r rune) rune {
			switch r {
			case '_', '-', ' ':
				return -1
			}
			return r
		}, k)
	}
	k = strings.ToLower(k)
	for _, s := range []string{"password", "passwd", "authorization", "cookie", "token", "secret", "privatekey", "credential", "dsn", "databaseurl", "apikey", "accesskey", "signingkey", "connectionstring", "debugerror", "stacktrace"} {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// Redact returns a recursively redacted copy of JSON-like data. Free text must not contain secrets.
func Redact(v any) any { return SafeValue(v) }
