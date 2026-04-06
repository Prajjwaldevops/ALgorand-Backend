package services

import (
	"fmt"
	"strings"
)

// MegaNZValidator validates mega.nz links (format-only, no HTTP checks)
type MegaNZValidator struct{}

// NewMegaNZValidator creates a new mega.nz link validator
func NewMegaNZValidator() *MegaNZValidator {
	return &MegaNZValidator{}
}


// ValidateMegaLink validates a mega.nz link by checking URL format.
// Note: We do NOT make HTTP requests to mega.nz because it's a JavaScript SPA
// that returns 400 Bad Request for HEAD/GET requests from server-side clients.
// URL format validation is sufficient for submission integrity.
func (v *MegaNZValidator) ValidateMegaLink(link string) error {
	link = strings.TrimSpace(link)

	// Format validation
	if link == "" {
		return fmt.Errorf("mega.nz link is required")
	}
	if !strings.HasPrefix(link, "https://mega.nz/") {
		return fmt.Errorf("invalid mega.nz link — must start with https://mega.nz/")
	}

	// Must contain a file or folder path after the domain
	path := strings.TrimPrefix(link, "https://mega.nz/")
	if len(path) < 5 {
		return fmt.Errorf("invalid mega.nz link — URL appears incomplete")
	}


	return nil
}


// ValidateEncryptionKeyContent validates that the uploaded .txt file
// contains an encryption key (non-empty, reasonable format)
func ValidateEncryptionKeyContent(content []byte) error {
	text := strings.TrimSpace(string(content))

	if len(text) == 0 {
		return fmt.Errorf("encryption key file is empty — must contain the decryption key")
	}

	if len(text) < 8 {
		return fmt.Errorf("encryption key too short — must be at least 8 characters")
	}

	if len(text) > 1024 {
		return fmt.Errorf("encryption key file too large — max 1024 characters")
	}

	return nil
}
