package auth

import (
	"errors"
	"regexp"

	"reflex-server/internal/models"
)

var (
	// RFC5322 regex for email validation
	emailRegex = regexp.MustCompile(`(?i)^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}$`)
	// E.164 regex for phone validation (e.g., +14155552671)
	phoneRegex = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)
)

func ValidateAndCategorizeIdentifier(identifier string) (models.IdentifierType, string, error) {
	if emailRegex.MatchString(identifier) {
		return models.IdentifierTypeEmail, identifier, nil
	}
	if phoneRegex.MatchString(identifier) {
		return models.IdentifierTypePhone, identifier, nil
	}
	return "", "", errors.New("invalid identifier: must be a valid email (RFC5322) or phone number in E.164 format (e.g. +1234567890)")
}
