package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"time"

	"bountyvault/internal/config"
	"bountyvault/internal/database"
	"bountyvault/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// AdminHandler handles admin panel endpoints (v3.1)
// Authentication is decoupled from Clerk — uses ADMIN_JWT_SECRET + bcrypt password
type AdminHandler struct {
	cfg *config.Config
}

func NewAdminHandler(cfg *config.Config) *AdminHandler {
	return &AdminHandler{cfg: cfg}
}

// POST /api/admin/login — Admin login with username/password
func (h *AdminHandler) Login(c *gin.Context) {
	var req models.AdminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid request"})
		return
	}

	// Check hardcoded admin credentials first (for bootstrap)
	if req.Username == h.cfg.AdminUsername && req.Password == h.cfg.AdminPassword {
		token, err := h.generateAdminJWT(req.Username, "bootstrap-admin")
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to generate token"})
			return
		}
		c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: models.AdminLoginResponse{
			Token:   token,
			Message: "Admin login successful",
			Admin:   models.AdminUser{Username: req.Username},
		}})
		return
	}

	// Check database admin_users table
	var admin models.AdminUser
	err := database.DB.QueryRow(`
		SELECT id, username, password_hash, display_name, last_login_at, created_at
		FROM admin_users WHERE username = $1
	`, req.Username).Scan(
		&admin.ID, &admin.Username, &admin.PasswordHash,
		&admin.DisplayName, &admin.LastLoginAt, &admin.CreatedAt,
	)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusUnauthorized, models.APIResponse{Success: false, Error: "Invalid credentials"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Database error"})
		return
	}

	// Verify bcrypt hash
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, models.APIResponse{Success: false, Error: "Invalid credentials"})
		return
	}

	// Update last login
	database.DB.Exec(`UPDATE admin_users SET last_login_at = NOW() WHERE id = $1`, admin.ID)

	token, err := h.generateAdminJWT(admin.Username, admin.ID.String())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: models.AdminLoginResponse{
		Token:   token,
		Message: "Admin login successful",
		Admin:   admin,
	}})
}

// GET /api/admin/stats — Aggregated platform statistics
func (h *AdminHandler) GetStats(c *gin.Context) {
	var stats models.AdminStats

	database.DB.QueryRow(`SELECT COUNT(*) FROM profiles WHERE role = 'freelancer'`).Scan(&stats.TotalFreelancers)
	database.DB.QueryRow(`SELECT COUNT(*) FROM profiles WHERE role = 'creator'`).Scan(&stats.TotalCreators)
	database.DB.QueryRow(`SELECT COUNT(*) FROM bounties`).Scan(&stats.TotalBounties)
	database.DB.QueryRow(`SELECT COUNT(*) FROM bounties WHERE status = 'open'`).Scan(&stats.OpenBounties)
	database.DB.QueryRow(`SELECT COUNT(*) FROM bounties WHERE status = 'in_progress'`).Scan(&stats.InProgressBounties)
	database.DB.QueryRow(`SELECT COUNT(*) FROM bounties WHERE status = 'completed'`).Scan(&stats.CompletedBounties)
	database.DB.QueryRow(`SELECT COUNT(*) FROM bounties WHERE status = 'disputed'`).Scan(&stats.DisputedBounties)
	database.DB.QueryRow(`SELECT COUNT(*) FROM submissions`).Scan(&stats.TotalSubmissions)
	database.DB.QueryRow(`SELECT COUNT(*) FROM submissions WHERE status = 'approved'`).Scan(&stats.AcceptedSubmissions)
	database.DB.QueryRow(`SELECT COALESCE(SUM(reward_algo), 0) FROM bounties`).Scan(&stats.TotalAlgoVolume)
	database.DB.QueryRow(`SELECT COALESCE(SUM(reward_algo), 0) FROM bounties WHERE status = 'completed'`).Scan(&stats.TotalAlgoPaidOut)
	database.DB.QueryRow(`SELECT COUNT(*) FROM disputes WHERE status = 'open'`).Scan(&stats.ActiveDisputes)
	database.DB.QueryRow(`SELECT COUNT(*) FROM transaction_log`).Scan(&stats.TotalTransactions)

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: stats})
}

// GET /api/admin/users — List all users with pagination
func (h *AdminHandler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	role := c.Query("role")
	search := c.Query("search")

	if page < 1 {
		page = 1
	}
	if pageSize > 100 {
		pageSize = 100
	}

	query := `
		SELECT id, clerk_id, username, email, display_name, avatar_url,
		       wallet_address, role, reputation_score, total_bounties_created,
		       total_bounties_completed, total_earned_algo, created_at
		FROM profiles WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM profiles WHERE 1=1`
	args := []interface{}{}
	idx := 1

	if role != "" {
		filter := ` AND role = $` + strconv.Itoa(idx)
		query += filter
		countQuery += filter
		args = append(args, role)
		idx++
	}
	if search != "" {
		filter := ` AND (username ILIKE $` + strconv.Itoa(idx) + ` OR email ILIKE $` + strconv.Itoa(idx) + `)`
		query += filter
		countQuery += filter
		args = append(args, "%"+search+"%")
		idx++
	}

	var total int
	database.DB.QueryRow(countQuery, args...).Scan(&total)

	query += ` ORDER BY created_at DESC LIMIT $` + strconv.Itoa(idx) + ` OFFSET $` + strconv.Itoa(idx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch users"})
		return
	}
	defer rows.Close()

	var users []gin.H
	for rows.Next() {
		var id, clerkID, username, email, role string
		var dn, av, wa *string
		var rep, bc, bm int
		var earned float64
		var createdAt time.Time
		rows.Scan(&id, &clerkID, &username, &email, &dn, &av, &wa, &role,
			&rep, &bc, &bm, &earned, &createdAt)
		users = append(users, gin.H{
			"id": id, "clerk_id": clerkID, "username": username, "email": email,
			"display_name": dn, "avatar_url": av, "wallet_address": wa, "role": role,
			"reputation_score": rep, "bounties_created": bc, "bounties_completed": bm,
			"total_earned_algo": earned, "created_at": createdAt,
		})
	}

	c.JSON(200, models.APIResponse{Success: true, Data: gin.H{
		"items": users, "total": total, "page": page, "page_size": pageSize,
	}})
}

// GET /api/admin/bounties — List all bounties for admin
func (h *AdminHandler) ListAllBounties(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")

	if page < 1 {
		page = 1
	}

	query := `
		SELECT b.id, b.bounty_id, b.title, b.reward_algo, b.status, b.deadline,
		       b.max_submissions, b.submissions_remaining, b.app_id, b.created_at,
		       p.username as creator_username,
		       (SELECT COUNT(*) FROM submissions s WHERE s.bounty_id = b.id) as sub_count
		FROM bounties b JOIN profiles p ON b.creator_id = p.id WHERE 1=1`
	args := []interface{}{}
	idx := 1

	if status != "" {
		query += ` AND b.status = $` + strconv.Itoa(idx)
		args = append(args, status)
		idx++
	}

	query += ` ORDER BY b.created_at DESC LIMIT $` + strconv.Itoa(idx) + ` OFFSET $` + strconv.Itoa(idx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch bounties"})
		return
	}
	defer rows.Close()

	var bounties []gin.H
	for rows.Next() {
		var id, bountyID, title, bStatus, creator string
		var reward float64
		var deadline, createdAt time.Time
		var maxS, remaining, subCount int
		var appID *int64
		rows.Scan(&id, &bountyID, &title, &reward, &bStatus, &deadline,
			&maxS, &remaining, &appID, &createdAt, &creator, &subCount)
		bounties = append(bounties, gin.H{
			"id": id, "bounty_id": bountyID, "title": title, "reward_algo": reward,
			"status": bStatus, "deadline": deadline, "max_submissions": maxS,
			"submissions_remaining": remaining, "app_id": appID,
			"created_at": createdAt, "creator_username": creator, "submission_count": subCount,
		})
	}

	c.JSON(200, models.APIResponse{Success: true, Data: bounties})
}

// GET /api/admin/transactions — List transaction log
func (h *AdminHandler) ListTransactions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	event := c.Query("event")

	if page < 1 {
		page = 1
	}

	query := `
		SELECT t.id, t.bounty_id, t.actor_id, t.event, t.txn_id, t.txn_note,
		       t.ipfs_metadata_cid, t.ipfs_gateway_url, t.amount_algo, t.created_at,
		       COALESCE(p.username, 'system') as actor_username
		FROM transaction_log t
		LEFT JOIN profiles p ON t.actor_id = p.id
		WHERE 1=1`
	args := []interface{}{}
	idx := 1

	if event != "" {
		query += ` AND t.event = $` + strconv.Itoa(idx)
		args = append(args, event)
		idx++
	}

	query += ` ORDER BY t.created_at DESC LIMIT $` + strconv.Itoa(idx) + ` OFFSET $` + strconv.Itoa(idx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch transactions"})
		return
	}
	defer rows.Close()

	var txns []gin.H
	for rows.Next() {
		var id, eventType, actorUsername string
		var bountyID, actorID, txnID, txnNote, ipfsCID, ipfsURL *string
		var amount *float64
		var createdAt time.Time
		rows.Scan(&id, &bountyID, &actorID, &eventType, &txnID, &txnNote,
			&ipfsCID, &ipfsURL, &amount, &createdAt, &actorUsername)
		txns = append(txns, gin.H{
			"id": id, "bounty_id": bountyID, "actor_id": actorID,
			"event": eventType, "txn_id": txnID, "txn_note": txnNote,
			"ipfs_metadata_cid": ipfsCID, "ipfs_gateway_url": ipfsURL,
			"amount_algo": amount, "created_at": createdAt,
			"actor_username": actorUsername,
		})
	}

	c.JSON(200, models.APIResponse{Success: true, Data: txns})
}

// GET /api/admin/disputes — List all disputes
func (h *AdminHandler) ListAllDisputes(c *gin.Context) {
	rows, err := database.DB.Query(`
		SELECT d.id, d.dispute_id, d.bounty_id, d.status,
		       d.votes_creator, d.votes_freelancer, d.voting_deadline, d.created_at,
		       b.title, b.reward_algo,
		       pf.username as freelancer_name,
		       pc.username as creator_name
		FROM disputes d
		JOIN bounties b ON d.bounty_id = b.id
		JOIN profiles pf ON d.freelancer_id = pf.id
		JOIN profiles pc ON d.creator_id = pc.id
		ORDER BY d.created_at DESC
		LIMIT 50
	`)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch disputes"})
		return
	}
	defer rows.Close()

	var disputes []gin.H
	for rows.Next() {
		var id, disputeID, bountyID, status, bTitle, freelancerName, creatorName string
		var vc, vf int
		var reward float64
		var votingDeadline, createdAt time.Time
		rows.Scan(&id, &disputeID, &bountyID, &status, &vc, &vf, &votingDeadline, &createdAt,
			&bTitle, &reward, &freelancerName, &creatorName)
		disputes = append(disputes, gin.H{
			"id": id, "dispute_id": disputeID, "bounty_id": bountyID, "status": status,
			"votes_creator": vc, "votes_freelancer": vf, "voting_deadline": votingDeadline,
			"created_at": createdAt, "bounty_title": bTitle, "reward_algo": reward,
			"freelancer_name": freelancerName, "creator_name": creatorName,
		})
	}

	c.JSON(200, models.APIResponse{Success: true, Data: disputes})
}

// GET /api/admin/audit-log — View audit log
func (h *AdminHandler) GetAuditLog(c *gin.Context) {
	rows, err := database.DB.Query(`
		SELECT id, admin_username, action, target_type, target_id, ip_address, created_at
		FROM audit_log ORDER BY created_at DESC LIMIT 100
	`)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch audit log"})
		return
	}
	defer rows.Close()

	var entries []gin.H
	for rows.Next() {
		var id, username, action string
		var targetType, targetID, ip *string
		var createdAt time.Time
		rows.Scan(&id, &username, &action, &targetType, &targetID, &ip, &createdAt)
		entries = append(entries, gin.H{
			"id": id, "admin_username": username, "action": action,
			"target_type": targetType, "target_id": targetID,
			"ip_address": ip, "created_at": createdAt,
		})
	}

	c.JSON(200, models.APIResponse{Success: true, Data: entries})
}

// ========================
// Helpers
// ========================

func (h *AdminHandler) generateAdminJWT(username, adminID string) (string, error) {
	claims := jwt.MapClaims{
		"sub":      adminID,
		"username": username,
		"role":     "admin",
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(h.cfg.AdminJWTSecret))
	if err != nil {
		log.Printf("[ERROR] Failed to sign admin JWT: %v", err)
		return "", err
	}

	return tokenString, nil
}
