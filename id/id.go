// Package id provides cryptographically generated UUIDs.
package id

import "github.com/google/uuid"

// UUID is a maintained UUID type with text and database serialization support.
type UUID = uuid.UUID

// NewV4 generates a UUID version 4 with cryptographic randomness.
func NewV4() (UUID, error) { return uuid.NewRandom() }

// NewV7 generates a time-ordered UUID version 7.
func NewV7() (UUID, error) { return uuid.NewV7() }

// Parse parses and validates an identifier.
func Parse(s string) (UUID, error) { return uuid.Parse(s) }

// Validate checks the supplied value according to package validation rules.
func Validate(s string) error { return uuid.Validate(s) }
