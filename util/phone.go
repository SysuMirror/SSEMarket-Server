package util

import "strings"

// NormalizePhone trims whitespace from legacy phone fields.
func NormalizePhone(phone string) string {
	return strings.TrimSpace(phone)
}

// IsQueryablePhone reports whether phone is safe to use in DB lookups.
// Empty strings, "0", and tombstone placeholders must never participate in WHERE phone=?.
func IsQueryablePhone(phone string) bool {
	phone = NormalizePhone(phone)
	if phone == "" || phone == "0" {
		return false
	}
	if strings.HasPrefix(phone, "deleted_") || strings.HasPrefix(phone, "legacy_") {
		return false
	}
	return true
}
