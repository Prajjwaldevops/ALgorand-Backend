package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode"
)

// SHA256Hash computes the SHA-256 hash of input data
func SHA256Hash(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// SHA256Hex computes the SHA-256 hash and returns hex string
func SHA256Hex(data []byte) string {
	return hex.EncodeToString(SHA256Hash(data))
}

// AlgorandAddressRegex validates Algorand address format (58 chars, base32)
var AlgorandAddressRegex = regexp.MustCompile(`^[A-Z2-7]{58}$`)

// ValidateAlgorandAddress checks if an address is a valid Algorand address
func ValidateAlgorandAddress(address string) bool {
	return AlgorandAddressRegex.MatchString(strings.TrimSpace(address))
}

// SanitizeString trims whitespace and removes control characters
func SanitizeString(s string) string {
	s = strings.TrimSpace(s)
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return -1
		}
		return r
	}, s)
}

// ValidateUUID checks if a string is a valid UUID v4
var UUIDRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func ValidateUUID(id string) bool {
	return UUIDRegex.MatchString(strings.ToLower(strings.TrimSpace(id)))
}
