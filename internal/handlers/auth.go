package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"

	"bountyvault/internal/config"
	"bountyvault/internal/database"
	"bountyvault/internal/models"
	"bountyvault/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AuthHandler struct {
	cfg *config.Config
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{cfg: cfg}
}

// POST /api/auth/sync — Auto-provisions a profile for a new Clerk user.
// Default role is always "freelancer" per v3.1 spec.
func (h *AuthHandler) SyncProfile(c *gin.Context) {
	clerkID, exists := c.Get("clerk_id")
	if !exists || clerkID == nil || clerkID == "" {
		log.Printf("[ERROR] SyncProfile: clerk_id not found in context")
		c.JSON(http.StatusUnauthorized, models.APIResponse{Success: false, Error: "Authentication required"})
		return
	}
	log.Printf("[INFO] SyncProfile called for clerk_id=%s", clerkID)

	var req models.SyncProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[WARN] SyncProfile: parse error for clerk_id=%s: %v", clerkID, err)
	}

	// Check if profile already exists
	var existingID string
	err := database.DB.QueryRow(`SELECT id FROM profiles WHERE clerk_id = $1`, clerkID).Scan(&existingID)
	if err == nil {
		// Profile exists — update role and name fields if provided in this request
		if req.Role != "" && (req.Role == "creator" || req.Role == "freelancer") {
			database.DB.Exec(`UPDATE profiles SET role = $1, updated_at = NOW() WHERE id = $2`, req.Role, existingID)
			log.Printf("[INFO] SyncProfile: updated role to '%s' for profile %s", req.Role, existingID)
		}
		// Update first/last/display name if provided
		if req.FirstName != "" || req.LastName != "" {
			displayName := req.FirstName
			if req.LastName != "" {
				displayName = req.FirstName + " " + req.LastName
			}
			database.DB.Exec(`UPDATE profiles SET display_name = $1, updated_at = NOW() WHERE id = $2`, displayName, existingID)
		}
		if req.Username != "" {
			database.DB.Exec(`UPDATE profiles SET username = $1, updated_at = NOW() WHERE id = $2`, req.Username, existingID)
		}
		pid, _ := uuid.Parse(existingID)
		profile := fetchProfile(pid)
		c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: profile})
		return
	}

	// If clerk_id is not found, the user might have recreated their Clerk account.
	// Check if a profile with the same email already exists.
	if req.Email != "" && req.Email != "unknown@bountyvault.com" {
		err = database.DB.QueryRow(`SELECT id FROM profiles WHERE email = $1`, req.Email).Scan(&existingID)
		if err == nil {
			// Link the new clerk_id to the existing profile
			database.DB.Exec(`UPDATE profiles SET clerk_id = $1, updated_at = NOW() WHERE id = $2`, clerkID, existingID)
			log.Printf("[INFO] SyncProfile: Re-linked new clerk_id %s to existing profile %s via email", clerkID, existingID)
			
			pid, _ := uuid.Parse(existingID)
			profile := fetchProfile(pid)
			c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: profile})
			return
		}
	}

	// Always default to "freelancer" unless explicitly set
	role := req.Role
	if role == "" || (role != "creator" && role != "freelancer") {
		role = "freelancer"
	}

	username := req.Username
	if username == "" {
		username = fmt.Sprintf("user_%s", clerkID.(string)[:8])
	}
	email := req.Email
	if email == "" {
		email = "unknown@bountyvault.com"
	}

	// Build display name
	displayName := username
	if req.FirstName != "" {
		displayName = req.FirstName
		if req.LastName != "" {
			displayName = req.FirstName + " " + req.LastName
		}
	}

	var newID uuid.UUID
	err = database.DB.QueryRow(`
		INSERT INTO profiles (clerk_id, email, username, display_name, role)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, clerkID, email, username, displayName, role).Scan(&newID)
	if err != nil {
		log.Printf("[ERROR] Failed to create profile for clerk_id=%s: %v", clerkID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to create profile: " + err.Error()})
		return
	}

	log.Printf("[OK] Profile created: id=%s clerk_id=%s email=%s username=%s role=%s", newID, clerkID, email, username, role)
	profile := fetchProfile(newID)
	c.JSON(http.StatusCreated, models.APIResponse{Success: true, Message: "Profile created", Data: profile})
}

// GET /api/auth/me
func (h *AuthHandler) GetMe(c *gin.Context) {
	profileIDStr, _ := c.Get("profile_id")
	pid, err := uuid.Parse(profileIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid profile ID"})
		return
	}

	profile := fetchProfile(pid)
	if profile.ID == uuid.Nil {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Profile not found"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: profile})
}

// PUT /api/auth/profile
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	profileIDStr, _ := c.Get("profile_id")

	var req models.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid request: " + err.Error()})
		return
	}

	if req.WalletAddress != nil && *req.WalletAddress != "" {
		if !utils.ValidateAlgorandAddress(*req.WalletAddress) {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid Algorand wallet address"})
			return
		}
	}

	query := `UPDATE profiles SET updated_at = NOW()`
	args := []interface{}{}
	argIdx := 1

	if req.DisplayName != nil {
		query += fmt.Sprintf(`, display_name = $%d`, argIdx)
		args = append(args, utils.SanitizeString(*req.DisplayName))
		argIdx++
	}
	if req.Bio != nil {
		query += fmt.Sprintf(`, bio = $%d`, argIdx)
		args = append(args, utils.SanitizeString(*req.Bio))
		argIdx++
	}
	if req.WalletAddress != nil {
		query += fmt.Sprintf(`, wallet_address = $%d`, argIdx)
		args = append(args, *req.WalletAddress)
		argIdx++
	}

	query += fmt.Sprintf(` WHERE id = $%d`, argIdx)
	args = append(args, profileIDStr)

	_, err := database.DB.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to update profile"})
		return
	}

	pid, _ := uuid.Parse(profileIDStr.(string))
	profile := fetchProfile(pid)
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Profile updated", Data: profile})
}

// PUT /api/auth/wallet
func (h *AuthHandler) LinkWallet(c *gin.Context) {
	profileIDStr, _ := c.Get("profile_id")

	var req struct {
		WalletAddress string `json:"wallet_address" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid request"})
		return
	}

	if !utils.ValidateAlgorandAddress(req.WalletAddress) {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid Algorand wallet address format"})
		return
	}

	var existingProfile string
	err := database.DB.QueryRow(
		`SELECT id FROM profiles WHERE wallet_address = $1 AND id != $2`,
		req.WalletAddress, profileIDStr,
	).Scan(&existingProfile)
	if err == nil {
		c.JSON(http.StatusConflict, models.APIResponse{Success: false, Error: "Wallet already linked to another account"})
		return
	}

	_, err = database.DB.Exec(
		`UPDATE profiles SET wallet_address = $1, updated_at = NOW() WHERE id = $2`,
		req.WalletAddress, profileIDStr,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to link wallet"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Wallet linked successfully"})
}

// PUT /api/auth/role — Switch role between freelancer and creator
func (h *AuthHandler) SwitchRole(c *gin.Context) {
	profileIDStr, _ := c.Get("profile_id")
	var req struct {
		Role string `json:"role" binding:"required,oneof=creator freelancer"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Role must be 'creator' or 'freelancer'"})
		return
	}

	_, err := database.DB.Exec(
		`UPDATE profiles SET role = $1, updated_at = NOW() WHERE id = $2`,
		req.Role, profileIDStr,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to switch role"})
		return
	}

	pid, _ := uuid.Parse(profileIDStr.(string))
	profile := fetchProfile(pid)
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Role updated to " + req.Role, Data: profile})
}

// ========================
// Helpers
// ========================

func fetchProfile(id uuid.UUID) models.Profile {
	var p models.Profile
	err := database.DB.QueryRow(`
		SELECT id, clerk_id, username, email,
		       display_name, avatar_url, wallet_address, COALESCE(role, 'freelancer'), bio,
		       COALESCE(reputation_score, 0), COALESCE(total_bounties_created, 0), COALESCE(total_bounties_completed, 0),
		       COALESCE(total_earned_algo, 0), COALESCE(streak_count, 0), COALESCE(avg_rating, 0), COALESCE(total_ratings, 0),
		       created_at, updated_at
		FROM profiles WHERE id = $1
	`, id).Scan(
		&p.ID, &p.ClerkID, &p.Username, &p.Email,
		&p.DisplayName, &p.AvatarURL, &p.WalletAddress, &p.Role, &p.Bio,
		&p.ReputationScore, &p.TotalBountiesCreated, &p.TotalBountiesCompleted,
		&p.TotalEarnedAlgo, &p.StreakCount, &p.AvgRating, &p.TotalRatings,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("[ERROR] fetchProfile(%s): %v", id, err)
	}
	return p
}

// trimPreview truncates text to maxLen characters with ellipsis
func trimPreview(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return strings.TrimSpace(text[:maxLen]) + "..."
}
