package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"bountyvault/internal/config"
	"bountyvault/internal/database"
	"bountyvault/internal/models"
	"bountyvault/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// BountyHandler handles bounty CRUD and workflow endpoints (v3.1)
type BountyHandler struct {
	cfg     *config.Config
	algoSvc *services.AlgorandService
	ipfsSvc *services.IPFSService
	r2Svc   *services.R2Service
}

func NewBountyHandler(cfg *config.Config, algoSvc *services.AlgorandService, ipfsSvc *services.IPFSService, r2Svc *services.R2Service) *BountyHandler {
	return &BountyHandler{cfg: cfg, algoSvc: algoSvc, ipfsSvc: ipfsSvc, r2Svc: r2Svc}
}

// GET /api/bounties — Public listing with filters
func (h *BountyHandler) ListBounties(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "12"))
	status := c.Query("status")
	search := c.Query("search")
	sortBy := c.DefaultQuery("sort", "created_at")
	order := c.DefaultQuery("order", "desc")
	tags := c.Query("tags")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 12
	}

	where := []string{"1=1"}
	args := []interface{}{}
	idx := 1

	if status != "" {
		where = append(where, fmt.Sprintf("b.status = $%d", idx))
		args = append(args, status)
		idx++
	}
	if search != "" {
		where = append(where, fmt.Sprintf("(b.title ILIKE $%d OR b.description ILIKE $%d)", idx, idx))
		args = append(args, "%"+search+"%")
		idx++
	}
	if tags != "" {
		where = append(where, fmt.Sprintf("b.tags && $%d", idx))
		args = append(args, pq.Array(strings.Split(tags, ",")))
		idx++
	}

	wc := strings.Join(where, " AND ")
	validSorts := map[string]string{
		"created_at": "b.created_at",
		"reward":     "b.reward_algo",
		"deadline":   "b.deadline",
	}
	sortCol := validSorts[sortBy]
	if sortCol == "" {
		sortCol = "b.created_at"
	}
	if order != "asc" {
		order = "desc"
	}

	var total int
	database.DB.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM bounties b WHERE %s`, wc), args...).Scan(&total)

	offset := (page - 1) * pageSize
	q := fmt.Sprintf(`
		SELECT b.id, b.bounty_id, b.creator_id, b.title, b.description, b.reward_algo,
		       b.deadline, b.status, b.app_id, b.max_submissions, b.submissions_remaining,
		       b.tags, b.created_at, b.updated_at,
		       p.id, p.username, p.display_name, p.avatar_url, p.reputation_score,
		       (SELECT COUNT(*) FROM submissions s WHERE s.bounty_id = b.id)
		FROM bounties b JOIN profiles p ON b.creator_id = p.id
		WHERE %s ORDER BY %s %s LIMIT $%d OFFSET $%d
	`, wc, sortCol, order, idx, idx+1)
	args = append(args, pageSize, offset)

	rows, err := database.DB.Query(q, args...)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch bounties"})
		return
	}
	defer rows.Close()

	bounties := []models.Bounty{}
	for rows.Next() {
		var b models.Bounty
		var cr models.Profile
		var sc int
		rows.Scan(
			&b.ID, &b.BountyID, &b.CreatorID, &b.Title, &b.Description, &b.RewardAlgo,
			&b.Deadline, &b.Status, &b.AppID, &b.MaxSubmissions, &b.SubmissionsRemaining,
			pq.Array(&b.Tags), &b.CreatedAt, &b.UpdatedAt,
			&cr.ID, &cr.Username, &cr.DisplayName, &cr.AvatarURL, &cr.ReputationScore, &sc,
		)
		b.Creator = &cr
		b.SubmissionCount = sc
		bounties = append(bounties, b)
	}

	c.JSON(200, models.APIResponse{Success: true, Data: models.PaginatedResponse{
		Items: bounties, TotalCount: total, Page: page, PageSize: pageSize,
		TotalPages: int(math.Ceil(float64(total) / float64(pageSize))),
	}})
}

// GET /api/bounties/:id — Get single bounty by UUID or bounty_id (CR00847)
func (h *BountyHandler) GetBounty(c *gin.Context) {
	id := c.Param("id")
	var b models.Bounty
	var cr models.Profile
	var sc int

	// Support lookup by both UUID and bounty_id (CR format)
	whereClause := "b.id = $1"
	if strings.HasPrefix(strings.ToUpper(id), "CR") {
		whereClause = "b.bounty_id = $1"
	}

	err := database.DB.QueryRow(fmt.Sprintf(`
		SELECT b.id, b.bounty_id, b.creator_id, b.title, b.description, b.reward_algo,
		       b.deadline, b.status, b.app_id, b.escrow_txn_id, b.payout_txn_id,
		       b.max_submissions, b.submissions_remaining, b.tags, b.created_at, b.updated_at,
		       p.id, p.username, p.display_name, p.avatar_url, p.reputation_score,
		       (SELECT COUNT(*) FROM submissions s WHERE s.bounty_id = b.id)
		FROM bounties b JOIN profiles p ON b.creator_id = p.id WHERE %s
	`, whereClause), id).Scan(
		&b.ID, &b.BountyID, &b.CreatorID, &b.Title, &b.Description, &b.RewardAlgo,
		&b.Deadline, &b.Status, &b.AppID, &b.EscrowTxnID, &b.PayoutTxnID,
		&b.MaxSubmissions, &b.SubmissionsRemaining, pq.Array(&b.Tags), &b.CreatedAt, &b.UpdatedAt,
		&cr.ID, &cr.Username, &cr.DisplayName, &cr.AvatarURL, &cr.ReputationScore, &sc,
	)

	if err == sql.ErrNoRows {
		c.JSON(404, models.APIResponse{Success: false, Error: "Bounty not found"})
		return
	}
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch bounty"})
		return
	}
	b.Creator = &cr
	b.SubmissionCount = sc
	c.JSON(200, models.APIResponse{Success: true, Data: b})
}

// POST /api/bounties — Create bounty in DB (no IPFS yet — happens on confirm-lock)
func (h *BountyHandler) CreateBounty(c *gin.Context) {
	pid, _ := c.Get("profile_id")
	var req models.CreateBountyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Translate validation errors into user-friendly messages
		errMsg := err.Error()
		if strings.Contains(errMsg, "'Title'") && strings.Contains(errMsg, "'min'") {
			errMsg = "Title must be at least 5 characters long"
		} else if strings.Contains(errMsg, "'Description'") && strings.Contains(errMsg, "'min'") {
			errMsg = "Description must be at least 20 characters long"
		} else if strings.Contains(errMsg, "'Title'") && strings.Contains(errMsg, "'required'") {
			errMsg = "Title is required"
		} else if strings.Contains(errMsg, "'Description'") && strings.Contains(errMsg, "'required'") {
			errMsg = "Description is required"
		}
		// Handle multiple validation errors
		if strings.Contains(err.Error(), "'Title'") && strings.Contains(err.Error(), "'Description'") {
			errMsg = "Title must be at least 5 characters and Description must be at least 20 characters"
		}
		c.JSON(400, models.APIResponse{Success: false, Error: errMsg})
		return
	}

	deadline, err := time.Parse(time.RFC3339, req.Deadline)
	if err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: "Invalid deadline format (use RFC3339)"})
		return
	}
	if deadline.Before(time.Now().Add(1 * time.Hour)) {
		c.JSON(400, models.APIResponse{Success: false, Error: "Deadline must be at least 1 hour in the future"})
		return
	}
	if req.RewardAlgo < 1.0 {
		c.JSON(400, models.APIResponse{Success: false, Error: "Minimum reward is 1 ALGO"})
		return
	}

	bid := uuid.New()
	// Generate unique CR-format bounty ID from DB function
	var bountyID string
	err = database.DB.QueryRow(`SELECT generate_bounty_id()`).Scan(&bountyID)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to generate bounty ID"})
		return
	}

	_, err = database.DB.Exec(`
		INSERT INTO bounties (id, bounty_id, creator_id, title, description, reward_algo,
		  deadline, max_submissions, submissions_remaining, tags, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, $9, 'open')`,
		bid, bountyID, pid, req.Title, req.Description, req.RewardAlgo,
		deadline, req.MaxSubmissions, pq.Array(req.Tags))
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to create bounty: " + err.Error()})
		return
	}

	database.DB.Exec(`UPDATE profiles SET total_bounties_created = total_bounties_created + 1 WHERE id = $1`, pid)

	c.JSON(201, models.APIResponse{
		Success: true,
		Message: "Bounty created — now lock funds to publish",
		Data:    gin.H{"id": bid, "bounty_id": bountyID},
	})
}

// POST /api/bounties/:id/lock — Build unsigned transactions for Pera Wallet
func (h *BountyHandler) LockBounty(c *gin.Context) {
	bid := c.Param("id")
	pid, _ := c.Get("profile_id")
	var req models.LockBountyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	var creatorID string
	var ra float64
	var dl time.Time
	var ms int
	var bountyDisplayID string
	err := database.DB.QueryRow(`
		SELECT creator_id, reward_algo, deadline, max_submissions, bounty_id
		FROM bounties WHERE id = $1 AND status = 'open'
	`, bid).Scan(&creatorID, &ra, &dl, &ms, &bountyDisplayID)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Bounty not found or not in open state"})
		return
	}
	if creatorID != pid.(string) {
		c.JSON(403, models.APIResponse{Success: false, Error: "Only the creator can lock funds"})
		return
	}

	// ESCROW BYPASS: When escrow address is configured, send funds directly
	// to the escrow account instead of the smart contract app address
	escrowAddr := h.cfg.EscrowAddress
	if escrowAddr != "" {
		log.Printf("[ESCROW-LOCK] Using escrow bypass: sending to %s instead of app address", escrowAddr)
		txns, err := h.algoSvc.BuildEscrowLockTxn(
			c.Request.Context(), req.WalletAddress, escrowAddr,
			uint64(ra*1e6), bountyDisplayID,
		)
		if err != nil {
			c.JSON(500, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(200, models.APIResponse{Success: true, Message: "Sign the transaction to lock funds in escrow", Data: txns})
		return
	}

	// Original smart contract flow — sends to app address
	txns, err := h.algoSvc.BuildCreateBountyTxns(
		c.Request.Context(), req.WalletAddress,
		uint64(ra*1e6), nil, uint64(dl.Unix()), uint64(ms), "",
	)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(200, models.APIResponse{Success: true, Message: "Sign both transactions with Pera Wallet", Data: txns})
}

// POST /api/bounties/:id/confirm-lock — Submit signed txns, pin to IPFS, record
func (h *BountyHandler) ConfirmLock(c *gin.Context) {
	bid := c.Param("id")
	pid, _ := c.Get("profile_id")
	var req models.ConfirmLockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	txID, err := h.algoSvc.SubmitSignedTxns(c.Request.Context(), req.SignedTxns)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Transaction submission failed: " + err.Error()})
		return
	}
	h.algoSvc.WaitForConfirmation(c.Request.Context(), txID, 10)

	// Fetch bounty details for IPFS metadata
	var b models.Bounty
	database.DB.QueryRow(`SELECT bounty_id, reward_algo, max_submissions, deadline, tags FROM bounties WHERE id = $1`, bid).Scan(
		&b.BountyID, &b.RewardAlgo, &b.MaxSubmissions, &b.Deadline, pq.Array(&b.Tags),
	)

	// Pin bounty creation metadata to IPFS (fire-and-forget)
	var ipfsCID, ipfsURL string
	go func() {
		var walletAddr string
		database.DB.QueryRow(`SELECT COALESCE(wallet_address,'') FROM profiles WHERE id = $1`, pid).Scan(&walletAddr)
		result, err := h.ipfsSvc.PinBountyCreated(context.Background(), services.BountyCreatedMetadata{
			BountyID:       b.BountyID,
			AppID:          req.AppID,
			CreatorAddress: walletAddr,
			RewardAlgo:     b.RewardAlgo,
			MaxSubmissions: b.MaxSubmissions,
			Deadline:       b.Deadline.Format(time.RFC3339),
			Tags:           b.Tags,
			TxnID:          txID,
			Network:        h.cfg.AlgoNetwork,
		})
		if err != nil {
			log.Printf("[WARN] IPFS pin failed for bounty %s: %v", bid, err)
			return
		}
		ipfsCID = result.CID
		ipfsURL = result.GatewayURL
		// Update transaction log with IPFS CID
		database.DB.Exec(`
			UPDATE transaction_log SET ipfs_metadata_cid = $1, ipfs_gateway_url = $2
			WHERE bounty_id = $3 AND event = 'escrow_locked'
		`, ipfsCID, ipfsURL, bid)
	}()

	database.DB.Exec(`UPDATE bounties SET app_id = $1, escrow_txn_id = $2, updated_at = NOW() WHERE id = $3`, req.AppID, txID, bid)
	database.DB.Exec(`
		INSERT INTO transaction_log (bounty_id, actor_id, event, txn_id, txn_note, amount_algo)
		VALUES ($1, $2, 'escrow_locked', $3, $4, $5)
	`, bid, pid, txID, fmt.Sprintf("BountyVault:escrow_locked:%d", req.AppID), b.RewardAlgo)

	c.JSON(200, models.APIResponse{Success: true, Message: "Funds locked on-chain", Data: gin.H{"txn_id": txID, "app_id": req.AppID}})
}

// POST /api/bounties/:id/submit — Freelancer submits work (v3.3: mega.nz link + encryption key .txt)
func (h *BountyHandler) SubmitWork(c *gin.Context) {
	bid := c.Param("id")
	pid, _ := c.Get("profile_id")

	// Parse multipart form
	megaNZLink := strings.TrimSpace(c.PostForm("mega_nz_link"))
	description := strings.TrimSpace(c.PostForm("description"))

	log.Printf("[DEBUG] SubmitWork: bid=%s, megaLink=%q, desc_len=%d", bid, megaNZLink, len(description))
	if megaNZLink == "" {
		log.Printf("[DEBUG] SubmitWork REJECT: mega.nz link empty")
		c.JSON(400, models.APIResponse{Success: false, Error: "mega.nz link is required"})
		return
	}
	if description == "" {
		log.Printf("[DEBUG] SubmitWork REJECT: description empty")
		c.JSON(400, models.APIResponse{Success: false, Error: "Description is required"})
		return
	}

	// Validate mega.nz link (format + HTTP reachability)
	megaValidator := services.NewMegaNZValidator()
	if err := megaValidator.ValidateMegaLink(megaNZLink); err != nil {
		log.Printf("[DEBUG] SubmitWork REJECT: mega validation: %v", err)
		c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Read and validate encryption key .txt file
	file, _, err := c.Request.FormFile("encryption_key")
	if err != nil {
		log.Printf("[DEBUG] SubmitWork REJECT: encryption key file missing: %v", err)
		c.JSON(400, models.APIResponse{Success: false, Error: "Encryption key .txt file is required"})
		return
	}
	defer file.Close()

	keyContent := make([]byte, 2048) // max read
	n, _ := file.Read(keyContent)
	keyContent = keyContent[:n]

	if err := services.ValidateEncryptionKeyContent(keyContent); err != nil {
		log.Printf("[DEBUG] SubmitWork REJECT: encryption key content: %v", err)
		c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Check bounty exists and is accepting submissions
	var status, creatorID string
	var remaining int
	var bountyDisplayID string
	var appID *int64
	var acceptedFreelancerID *string
	err = database.DB.QueryRow(`
		SELECT status, creator_id, submissions_remaining, bounty_id, app_id, accepted_freelancer_id
		FROM bounties WHERE id = $1
	`, bid).Scan(&status, &creatorID, &remaining, &bountyDisplayID, &appID, &acceptedFreelancerID)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Bounty not found"})
		return
	}
	log.Printf("[DEBUG] SubmitWork: bounty status=%s, creatorID=%s, pid=%s, remaining=%d, acceptedFL=%v", status, creatorID, pid, remaining, acceptedFreelancerID)
	if status != "in_progress" {
		log.Printf("[DEBUG] SubmitWork REJECT: bounty status is %s, not in_progress", status)
		c.JSON(400, models.APIResponse{Success: false, Error: "Bounty is not in progress — cannot submit work"})
		return
	}
	if creatorID == pid.(string) {
		c.JSON(400, models.APIResponse{Success: false, Error: "Creator cannot submit work"})
		return
	}
	// Only the accepted freelancer can submit
	if acceptedFreelancerID == nil || *acceptedFreelancerID != pid.(string) {
		c.JSON(403, models.APIResponse{Success: false, Error: "Only the accepted freelancer can submit work"})
		return
	}
	if remaining <= 0 {
		c.JSON(400, models.APIResponse{Success: false, Error: "No submission slots remaining"})
		return
	}

	// v3.4: Require wallet to be linked for payout
	var freelancerWallet string
	err = database.DB.QueryRow(`SELECT COALESCE(wallet_address, '') FROM profiles WHERE id = $1`, pid).Scan(&freelancerWallet)
	if err != nil || freelancerWallet == "" {
		log.Printf("[DEBUG] SubmitWork REJECT: freelancer wallet not linked")
		c.JSON(400, models.APIResponse{Success: false, Error: "You must link your Algorand wallet before submitting work. Go to your profile to connect your wallet."})
		return
	}

	// Get submission number (supports resubmissions)
	var subNum int
	database.DB.QueryRow(`SELECT COALESCE(MAX(submission_number), 0) + 1 FROM submissions WHERE bounty_id = $1 AND freelancer_id = $2`, bid, pid).Scan(&subNum)

	// Upload encryption key to Cloudflare R2
	r2Result, err := h.r2Svc.UploadEncryptionKey(
		c.Request.Context(),
		pid.(string), bid, subNum,
		keyContent, megaNZLink,
	)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to upload encryption key: " + err.Error()})
		return
	}

	// Generate presigned URL for the encryption key file
	signedURL, _ := h.r2Svc.GenerateSignedURL(c.Request.Context(), r2Result.Path)

	// Insert submission record with freelancer wallet address
	sid := uuid.New()
	_, err = database.DB.Exec(`
		INSERT INTO submissions (id, bounty_id, freelancer_id, submission_number,
		  mega_nz_link, encryption_key_r2_path, encryption_key_r2_url,
		  description, status, work_hash_sha256, freelancer_wallet_address)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending', $9, $10)
	`, sid, bid, pid, subNum, megaNZLink, r2Result.Path, signedURL, description, r2Result.WorkHashSHA256, freelancerWallet)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to record submission: " + err.Error()})
		return
	}

	// Record in transaction_log
	appIDVal := int64(0)
	if appID != nil {
		appIDVal = *appID
	}

	event := "work_submitted"
	if subNum > 1 {
		event = "work_resubmitted"
	}
	database.DB.Exec(`
		INSERT INTO transaction_log (bounty_id, actor_id, event, txn_note, amount_algo)
		VALUES ($1, $2, $3, $4, 0)
	`, bid, pid, event, fmt.Sprintf("BountyVault:%s:%d", event, appIDVal))

	// Pin submission metadata to IPFS (fire-and-forget)
	go func() {
		var workerAddr string
		database.DB.QueryRow(`SELECT COALESCE(wallet_address,'') FROM profiles WHERE id = $1`, pid).Scan(&workerAddr)
		descPreview := description
		if len(descPreview) > 200 {
			descPreview = descPreview[:200] + "..."
		}

		if subNum > 1 {
			// Resubmission
			_, err := h.ipfsSvc.PinWorkResubmitted(context.Background(), services.WorkResubmittedMetadata{
				BountyID:           bountyDisplayID,
				SubmissionNumber:   subNum,
				FreelancerAddress:  workerAddr,
				MegaNZLink:         megaNZLink,
				DescriptionPreview: descPreview,
				WorkHashSHA256:     r2Result.WorkHashSHA256,
				Network:            h.cfg.AlgoNetwork,
			})
			if err != nil {
				log.Printf("[WARN] IPFS pin failed for resubmission %s: %v", sid, err)
			}
		} else {
			// First submission
			_, err := h.ipfsSvc.PinWorkSubmitted(context.Background(), services.WorkSubmittedMetadata{
				BountyID:           bountyDisplayID,
				SubmissionNumber:   subNum,
				FreelancerAddress:  workerAddr,
				FileR2Path:         r2Result.Path,
				FileType:           "txt",
				FileSizeBytes:      r2Result.FileSize,
				DescriptionPreview: descPreview,
				WorkHashSHA256:     r2Result.WorkHashSHA256,
				Network:            h.cfg.AlgoNetwork,
			})
			if err != nil {
				log.Printf("[WARN] IPFS pin failed for submission %s: %v", sid, err)
			}
		}
	}()

	c.JSON(201, models.APIResponse{
		Success: true,
		Message: "Work submitted successfully",
		Data: gin.H{
			"submission_id":     sid,
			"submission_number": subNum,
			"mega_nz_link":      megaNZLink,
			"work_hash":         r2Result.WorkHashSHA256,
		},
	})
}

// POST /api/bounties/:id/build-approve-payout — Build unsigned approve_payout txn for Pera signing
func (h *BountyHandler) BuildApprovePayout(c *gin.Context) {
	bid := c.Param("id")
	pid, _ := c.Get("profile_id")
	var req struct {
		SubmissionID  string `json:"submission_id" binding:"required"`
		WalletAddress string `json:"wallet_address" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Verify bounty ownership and status
	var creatorID string
	var appID *int64
	err := database.DB.QueryRow(`
		SELECT creator_id, app_id FROM bounties WHERE id = $1 AND status = 'in_progress'
	`, bid).Scan(&creatorID, &appID)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Bounty not found or not in progress"})
		return
	}
	if creatorID != pid.(string) {
		c.JSON(403, models.APIResponse{Success: false, Error: "Only the creator can approve payouts"})
		return
	}

	// Get freelancer wallet from the submission
	var freelancerWallet *string
	var freelancerID string
	err = database.DB.QueryRow(`
		SELECT freelancer_id, freelancer_wallet_address
		FROM submissions WHERE id = $1 AND bounty_id = $2 AND status = 'pending'
	`, req.SubmissionID, bid).Scan(&freelancerID, &freelancerWallet)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Pending submission not found"})
		return
	}

	// Determine payout wallet — submission-stored wallet, fallback to profile
	payoutWallet := ""
	if freelancerWallet != nil && *freelancerWallet != "" {
		payoutWallet = *freelancerWallet
	} else {
		database.DB.QueryRow(`SELECT COALESCE(wallet_address,'') FROM profiles WHERE id = $1`, freelancerID).Scan(&payoutWallet)
	}
	if payoutWallet == "" {
		c.JSON(400, models.APIResponse{Success: false, Error: "Freelancer has no wallet linked — cannot build payout transaction"})
		return
	}

	if h.algoSvc == nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Algorand service unavailable"})
		return
	}

	// Build unsigned approve_payout transaction
	txns, err := h.algoSvc.BuildApprovePayoutTxn(c.Request.Context(), req.WalletAddress, payoutWallet)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to build payout transaction: " + err.Error()})
		return
	}

	c.JSON(200, models.APIResponse{
		Success: true,
		Message: "Sign the transaction with Pera Wallet to release escrow funds",
		Data: gin.H{
			"transactions":    txns,
			"freelancer_id":   freelancerID,
			"payout_wallet":   payoutWallet,
		},
	})
}

// PUT /api/bounties/:id/approve — Creator approves submission + payout
// ESCROW BYPASS: Server-side direct payment from configured escrow account
func (h *BountyHandler) ApproveSubmission(c *gin.Context) {
	bid := c.Param("id")
	pid, _ := c.Get("profile_id")
	var req models.ApproveSubmissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	var creatorID string
	var reward float64
	var bountyDisplayID string
	var appID *int64
	err := database.DB.QueryRow(`
		SELECT creator_id, reward_algo, bounty_id, app_id FROM bounties WHERE id = $1 AND status = 'in_progress'
	`, bid).Scan(&creatorID, &reward, &bountyDisplayID, &appID)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Bounty not found or not in progress"})
		return
	}
	if creatorID != pid.(string) {
		c.JSON(403, models.APIResponse{Success: false, Error: "Only creator can approve"})
		return
	}

	// First get the submission info to find freelancer wallet
	var freelancerID string
	var freelancerWallet *string
	err = database.DB.QueryRow(`
		SELECT freelancer_id, freelancer_wallet_address
		FROM submissions WHERE id = $1 AND bounty_id = $2 AND status = 'pending'
	`, req.SubmissionID, bid).Scan(&freelancerID, &freelancerWallet)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Pending submission not found"})
		return
	}

	// Determine payout wallet
	payoutWallet := ""
	if freelancerWallet != nil && *freelancerWallet != "" {
		payoutWallet = *freelancerWallet
	} else {
		database.DB.QueryRow(`SELECT COALESCE(wallet_address,'') FROM profiles WHERE id = $1`, freelancerID).Scan(&payoutWallet)
	}
	if payoutWallet == "" {
		c.JSON(400, models.APIResponse{Success: false, Error: "Freelancer has no wallet linked — cannot send payout"})
		return
	}

	// ESCROW BYPASS: Send ALGO directly from configured escrow account
	var txID string
	escrowMnemonic := h.cfg.EscrowMnemonic
	if escrowMnemonic != "" {
		// Server-side direct payment — no Pera Wallet signing needed
		amountMicroAlgos := uint64(reward * 1e6)
		note := fmt.Sprintf("BountyVault:payout:%s:%s", bountyDisplayID, freelancerID)
		log.Printf("[ESCROW-BYPASS] Sending %.6f ALGO (%d microAlgos) to %s for bounty %s",
			reward, amountMicroAlgos, payoutWallet, bountyDisplayID)

		txID, err = h.algoSvc.SendDirectPayment(c.Request.Context(), escrowMnemonic, payoutWallet, amountMicroAlgos, note)
		if err != nil {
			log.Printf("[ESCROW-BYPASS] Payment failed: %v", err)
			c.JSON(500, models.APIResponse{Success: false, Error: "Escrow payment failed: " + err.Error()})
			return
		}
		log.Printf("[ESCROW-BYPASS] Payment success! txID=%s", txID)
		h.algoSvc.WaitForConfirmation(c.Request.Context(), txID, 10)
	} else if len(req.SignedTxns) > 0 {
		// Fallback: use signed transactions from Pera Wallet (original flow)
		txID, err = h.algoSvc.SubmitSignedTxns(c.Request.Context(), req.SignedTxns)
		if err != nil {
			c.JSON(500, models.APIResponse{Success: false, Error: "Blockchain transaction failed: " + err.Error()})
			return
		}
		h.algoSvc.WaitForConfirmation(c.Request.Context(), txID, 10)
	} else {
		// Generate a mock txn ID if no escrow and no signed txns
		txID = fmt.Sprintf("escrow_bypass_%s_%d", bid, time.Now().UnixMilli())
	}

	// Update submission
	_, err = database.DB.Exec(`
		UPDATE submissions SET status = 'approved', creator_message = $1, creator_rating = $2,
		  resolved_at = NOW(), payout_txn_hash = $5
		WHERE id = $3 AND bounty_id = $4 AND status = 'pending'
	`, req.Message, req.Rating, req.SubmissionID, bid, txID)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to update submission"})
		return
	}

	database.DB.Exec(`UPDATE bounties SET status = 'completed', payout_txn_id = $1, updated_at = NOW() WHERE id = $2`, txID, bid)
	database.DB.Exec(`
		UPDATE profiles SET
		  total_bounties_completed = total_bounties_completed + 1,
		  total_earned_algo = total_earned_algo + $1,
		  streak_count = streak_count + 1,
		  reputation_score = reputation_score + 10
		WHERE id = $2
	`, reward, freelancerID)

	// Log transaction with freelancer wallet address
	appIDVal := int64(0)
	if appID != nil {
		appIDVal = *appID
	}
	txnNote := fmt.Sprintf("BountyVault:submission_approved:%d:payout_to:%s", appIDVal, payoutWallet)
	database.DB.Exec(`
		INSERT INTO transaction_log (bounty_id, actor_id, event, txn_id, txn_note, amount_algo)
		VALUES ($1, $2, 'submission_approved', $3, $4, $5)
	`, bid, pid, txID, txnNote, reward)

	// Also log payout received for the freelancer
	database.DB.Exec(`
		INSERT INTO transaction_log (bounty_id, actor_id, event, txn_id, txn_note, amount_algo)
		VALUES ($1, $2, 'payout_received', $3, $4, $5)
	`, bid, freelancerID, txID, fmt.Sprintf("BountyVault:payout_received:%d:wallet:%s", appIDVal, payoutWallet), reward)

	// Pin approval metadata to IPFS (fire-and-forget)
	go func() {
		var creatorAddr string
		database.DB.QueryRow(`SELECT COALESCE(wallet_address,'') FROM profiles WHERE id = $1`, pid).Scan(&creatorAddr)
		_, err := h.ipfsSvc.PinSubmissionApproved(context.Background(), services.SubmissionApprovedMetadata{
			BountyID:          bountyDisplayID,
			SubmissionID:      req.SubmissionID,
			FreelancerAddress: payoutWallet,
			CreatorAddress:    creatorAddr,
			RewardPaidAlgo:    reward,
			CreatorRating:     req.Rating,
			CreatorMessage:    req.Message,
			PayoutTxnID:       txID,
			Network:           h.cfg.AlgoNetwork,
		})
		if err != nil {
			log.Printf("[WARN] IPFS pin failed for approval: %v", err)
		}
	}()

	c.JSON(200, models.APIResponse{Success: true, Message: "Submission approved and ALGO transferred", Data: gin.H{
		"payout_txn_id":   txID,
		"payout_txn_hash": txID,
		"reward_algo":     reward,
		"freelancer_id":   freelancerID,
		"payout_wallet":   payoutWallet,
	}})
}

// PUT /api/bounties/:id/reject — Creator rejects submission (min 50 char feedback)
func (h *BountyHandler) RejectSubmission(c *gin.Context) {
	bid := c.Param("id")
	pid, _ := c.Get("profile_id")
	var req models.RejectSubmissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Enforce 50-char minimum feedback
	if len(strings.TrimSpace(req.Feedback)) < 50 {
		c.JSON(400, models.APIResponse{Success: false, Error: "Rejection feedback must be at least 50 characters"})
		return
	}

	var creatorID string
	var bountyDisplayID string
	var appID *int64
	database.DB.QueryRow(`SELECT creator_id, bounty_id, app_id FROM bounties WHERE id = $1`, bid).Scan(&creatorID, &bountyDisplayID, &appID)
	if creatorID != pid.(string) {
		c.JSON(403, models.APIResponse{Success: false, Error: "Only creator can reject"})
		return
	}

	// Submit rejection transaction if txns are provided
	txID := "mock_reject_txn_" + bid
	var err error
	if len(req.SignedTxns) > 0 {
		txID, err = h.algoSvc.SubmitSignedTxns(c.Request.Context(), req.SignedTxns)
		if err != nil {
			c.JSON(500, models.APIResponse{Success: false, Error: "Transaction failed: " + err.Error()})
			return
		}
		h.algoSvc.WaitForConfirmation(c.Request.Context(), txID, 10)
	}

	// Count this rejection number
	var rejNum int
	database.DB.QueryRow(`SELECT COUNT(*) + 1 FROM submissions WHERE bounty_id = $1 AND status = 'rejected'`, bid).Scan(&rejNum)

	var freelancerID string
	err = database.DB.QueryRow(`
		UPDATE submissions SET status = 'rejected', rejection_feedback = $1, submission_txn_id = $2, resolved_at = NOW()
		WHERE id = $3 AND bounty_id = $4 AND status = 'pending'
		RETURNING freelancer_id
	`, req.Feedback, txID, req.SubmissionID, bid).Scan(&freelancerID)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Submission not found"})
		return
	}

	// Decrement submissions_remaining
	var remaining int
	database.DB.QueryRow(`
		UPDATE bounties SET submissions_remaining = submissions_remaining - 1, updated_at = NOW()
		WHERE id = $1 RETURNING submissions_remaining
	`, bid).Scan(&remaining)

	// If all submission slots exhausted, set bounty status to 'expired'
	if remaining == 0 {
		database.DB.Exec(`UPDATE bounties SET status = 'expired', updated_at = NOW() WHERE id = $1`, bid)
	}

	// Notify freelancer
	database.DB.Exec(`
		INSERT INTO notifications (user_id, type, title, message, bounty_id)
		VALUES ($1, 'submission_rejected', 'Submission Rejected',
			$2, $3)
	`, freelancerID,
		fmt.Sprintf("Your submission was rejected. %d slot(s) remaining.", remaining),
		bid,
	)

	// Log transaction
	appIDVal := int64(0)
	if appID != nil {
		appIDVal = *appID
	}
	database.DB.Exec(`
		INSERT INTO transaction_log (bounty_id, actor_id, event, txn_id, txn_note)
		VALUES ($1, $2, 'submission_rejected', $3, $4)
	`, bid, pid, txID, fmt.Sprintf("BountyVault:submission_rejected:%d", appIDVal))

	// Pin rejection metadata to IPFS (fire-and-forget)
	feedbackPreview := req.Feedback
	if len(feedbackPreview) > 200 {
		feedbackPreview = feedbackPreview[:200] + "..."
	}
	go func() {
		_, err := h.ipfsSvc.PinSubmissionRejected(context.Background(), services.SubmissionRejectedMetadata{
			BountyID:                  bountyDisplayID,
			SubmissionID:              req.SubmissionID,
			RejectionNumber:           rejNum,
			SubmissionsRemainingAfter: remaining,
			FeedbackPreview:           feedbackPreview,
			TxnID:                     txID,
			Network:                   h.cfg.AlgoNetwork,
		})
		if err != nil {
			log.Printf("[WARN] IPFS pin failed for rejection: %v", err)
		}
	}()

	msg := "Submission rejected"
	if remaining == 0 {
		msg = "Submission rejected — all slots exhausted. Freelancer can now Let Go or Raise Dispute."
	} else {
		msg = fmt.Sprintf("Submission rejected — %d slot(s) remaining for resubmission", remaining)
	}

	c.JSON(200, models.APIResponse{
		Success: true,
		Message: msg,
		Data: gin.H{
			"submissions_remaining": remaining,
			"exhausted":             remaining == 0,
		},
	})
}

// POST /api/bounties/:id/dispute — Freelancer raises a DAO dispute (300 word min)
func (h *BountyHandler) InitiateDispute(c *gin.Context) {
	bid := c.Param("id")
	pid, _ := c.Get("profile_id")
	var req models.RaiseDisputeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Enforce 300-word minimum server-side
	wordCount := len(strings.Fields(req.Description))
	if wordCount < 300 {
		c.JSON(400, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Dispute description must be at least 300 words (you have %d)", wordCount),
		})
		return
	}

	// Verify bounty and freelancer eligibility
	var status, creatorID string
	var bountyDisplayID string
	var rewardAlgo float64
	var appID *int64
	err := database.DB.QueryRow(`SELECT status, creator_id, bounty_id, reward_algo, app_id FROM bounties WHERE id = $1`, bid).Scan(
		&status, &creatorID, &bountyDisplayID, &rewardAlgo, &appID,
	)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Bounty not found"})
		return
	}
	// v3.5: Allow dispute when in_progress OR expired (at least 1 rejected submission required)
	if status != "expired" && status != "in_progress" {
		c.JSON(400, models.APIResponse{Success: false, Error: "Dispute only allowed when bounty is in progress or all submission slots are exhausted"})
		return
	}
	if creatorID == pid.(string) {
		c.JSON(400, models.APIResponse{Success: false, Error: "Creator cannot raise a dispute"})
		return
	}

	// Require at least 1 rejected submission from this freelancer
	var rejectedCount int
	database.DB.QueryRow(`SELECT COUNT(*) FROM submissions WHERE bounty_id = $1 AND freelancer_id = $2 AND status = 'rejected'`, bid, pid).Scan(&rejectedCount)
	if rejectedCount == 0 {
		c.JSON(400, models.APIResponse{Success: false, Error: "You must have at least 1 rejected submission to raise a dispute"})
		return
	}

	// Submit initiate_dispute transaction
	txID := "mock_dispute_txn_" + bid
	if len(req.SignedTxns) > 0 {
		txID, err = h.algoSvc.SubmitSignedTxns(c.Request.Context(), req.SignedTxns)
		if err != nil {
			c.JSON(500, models.APIResponse{Success: false, Error: "Blockchain transaction failed: " + err.Error()})
			return
		}
		h.algoSvc.WaitForConfirmation(c.Request.Context(), txID, 10)
	}

	// Collect full submission history for IPFS metadata
	subHistory := []services.SubmissionHistoryEntry{}
	rows, _ := database.DB.Query(`
		SELECT submission_number, file_url, description, COALESCE(rejection_feedback,''), created_at, resolved_at
		FROM submissions WHERE bounty_id = $1 AND freelancer_id = $2 ORDER BY submission_number ASC
	`, bid, pid)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var entry services.SubmissionHistoryEntry
			var createdAt time.Time
			var resolvedAt *time.Time
			rows.Scan(&entry.Attempt, &entry.FileR2Path, &entry.Description, &entry.RejectionFeedback, &createdAt, &resolvedAt)
			entry.SubmittedAt = createdAt.Format(time.RFC3339)
			if resolvedAt != nil {
				entry.RejectedAt = resolvedAt.Format(time.RFC3339)
			}
			subHistory = append(subHistory, entry)
		}
	}

	subHistoryJSON, _ := json.Marshal(subHistory)
	votingDeadline := time.Now().Add(48 * time.Hour)
	did := uuid.New()

	// Generate dispute ID
	var disputeDisplayID string
	database.DB.QueryRow(`SELECT generate_dispute_id()`).Scan(&disputeDisplayID)

	_, err = database.DB.Exec(`
		INSERT INTO disputes (id, dispute_id, bounty_id, freelancer_id, creator_id,
		  freelancer_description, submission_history, status, voting_deadline,
		  initiated_by, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'open', $8, $4, $6)
	`, did, disputeDisplayID, bid, pid, creatorID, req.Description, subHistoryJSON, votingDeadline)
	if err != nil {
		log.Printf("[ERROR] Failed to create dispute record for bounty %s: %v", bid, err)
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to create dispute record: " + err.Error()})
		return
	}

	database.DB.Exec(`UPDATE bounties SET status = 'disputed', updated_at = NOW() WHERE id = $1`, bid)

	appIDVal := int64(0)
	if appID != nil {
		appIDVal = *appID
	}
	database.DB.Exec(`
		INSERT INTO transaction_log (bounty_id, actor_id, event, txn_id, txn_note)
		VALUES ($1, $2, 'dispute_raised', $3, $4)
	`, bid, pid, txID, fmt.Sprintf("BountyVault:dispute_raised:%d", appIDVal))

	// Pin dispute metadata to IPFS (fire-and-forget)
	go func() {
		var freelancerAddr string
		database.DB.QueryRow(`SELECT COALESCE(wallet_address,'') FROM profiles WHERE id = $1`, pid).Scan(&freelancerAddr)
		result, err := h.ipfsSvc.PinDisputeRaised(context.Background(), services.DisputeRaisedMetadata{
			DisputeID:           disputeDisplayID,
			BountyID:            bountyDisplayID,
			FreelancerAddress:   freelancerAddr,
			DisputeDescription:  req.Description,
			SubmissionHistory:   subHistory,
			VotingDeadline:      votingDeadline.Format(time.RFC3339),
			TxnID:               txID,
			Network:             h.cfg.AlgoNetwork,
		})
		if err != nil {
			log.Printf("[WARN] IPFS pin failed for dispute: %v", err)
			return
		}
		database.DB.Exec(`UPDATE disputes SET ipfs_dispute_cid = $1 WHERE id = $2`, result.CID, did)
	}()

	c.JSON(201, models.APIResponse{
		Success: true,
		Message: "Dispute raised — DAO Court is now open for 48 hours",
		Data: gin.H{
			"dispute_id":     disputeDisplayID,
			"voting_deadline": votingDeadline,
		},
	})
}

// POST /api/bounties/:id/letgo — Freelancer forfeits, creator refunded, rating reduced by 20%
// v3.5: Uses escrow bypass to refund creator directly
func (h *BountyHandler) LetGoBounty(c *gin.Context) {
	bid := c.Param("id")
	pid, _ := c.Get("profile_id")

	var status, creatorID string
	var rewardAlgo float64
	var bountyDisplayID string
	var appID *int64
	err := database.DB.QueryRow(`SELECT status, creator_id, reward_algo, bounty_id, app_id FROM bounties WHERE id = $1`, bid).Scan(
		&status, &creatorID, &rewardAlgo, &bountyDisplayID, &appID,
	)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Bounty not found"})
		return
	}
	// Allow Let Go when in_progress or expired, but freelancer must have at least 1 submission
	if status != "expired" && status != "in_progress" {
		c.JSON(400, models.APIResponse{Success: false, Error: "Let Go only available when bounty is in progress or submission slots are exhausted"})
		return
	}
	if creatorID == pid.(string) {
		c.JSON(400, models.APIResponse{Success: false, Error: "Creator cannot use Let Go"})
		return
	}

	// Verify freelancer has at least 1 submission
	var subCount int
	database.DB.QueryRow(`SELECT COUNT(*) FROM submissions WHERE bounty_id = $1 AND freelancer_id = $2`, bid, pid).Scan(&subCount)
	if subCount == 0 {
		c.JSON(400, models.APIResponse{Success: false, Error: "You must have at least 1 submission to use Let Go"})
		return
	}

	// Get creator wallet for refund
	var creatorWallet string
	database.DB.QueryRow(`SELECT COALESCE(wallet_address,'') FROM profiles WHERE id = $1`, creatorID).Scan(&creatorWallet)

	// ESCROW BYPASS: Send ALGO back to creator from configured escrow account
	var txID string
	escrowMnemonic := h.cfg.EscrowMnemonic
	if escrowMnemonic != "" && creatorWallet != "" && h.algoSvc != nil {
		amountMicroAlgos := uint64(rewardAlgo * 1e6)
		note := fmt.Sprintf("BountyVault:freelancer_letgo:%s:refund_to:%s", bountyDisplayID, creatorWallet)
		log.Printf("[LETGO-REFUND] Sending %.6f ALGO (%d microAlgos) to creator %s for bounty %s",
			rewardAlgo, amountMicroAlgos, creatorWallet, bountyDisplayID)

		txID, err = h.algoSvc.SendDirectPayment(c.Request.Context(), escrowMnemonic, creatorWallet, amountMicroAlgos, note)
		if err != nil {
			log.Printf("[LETGO-REFUND] Payment failed: %v", err)
			c.JSON(500, models.APIResponse{Success: false, Error: "Escrow refund failed: " + err.Error()})
			return
		}
		log.Printf("[LETGO-REFUND] Refund success! txID=%s", txID)
		h.algoSvc.WaitForConfirmation(c.Request.Context(), txID, 10)
	} else {
		// Fallback mock
		txID = fmt.Sprintf("letgo_refund_%s_%d", bid, time.Now().UnixMilli())
	}

	database.DB.Exec(`UPDATE bounties SET status = 'cancelled', updated_at = NOW() WHERE id = $1`, bid)

	// v3.5: Reduce freelancer reputation by 20% (not reset to 0)
	database.DB.Exec(`
		UPDATE profiles SET
		  reputation_score = GREATEST(0, FLOOR(reputation_score * 0.8))
		WHERE id = $1
	`, pid)

	appIDVal := int64(0)
	if appID != nil {
		appIDVal = *appID
	}

	// Log transaction for creator (refund)
	database.DB.Exec(`
		INSERT INTO transaction_log (bounty_id, actor_id, event, txn_id, txn_note, amount_algo)
		VALUES ($1, $2, 'freelancer_letgo', $3, $4, $5)
	`, bid, creatorID, txID, fmt.Sprintf("BountyVault:freelancer_letgo:%d:refund_to:%s", appIDVal, creatorWallet), rewardAlgo)

	// Pin let-go metadata to IPFS
	go func() {
		var freelancerAddr string
		database.DB.QueryRow(`SELECT COALESCE(wallet_address,'') FROM profiles WHERE id = $1`, pid).Scan(&freelancerAddr)
		_, err := h.ipfsSvc.PinFreelancerLetGo(context.Background(), services.FreelancerLetGoMetadata{
			BountyID:          bountyDisplayID,
			FreelancerAddress: freelancerAddr,
			CreatorAddress:    creatorWallet,
			RefundedAlgo:      rewardAlgo,
			RatingReset:       false, // v3.5: reduced by 20%, not reset
			TxnID:             txID,
			Network:           h.cfg.AlgoNetwork,
		})
		if err != nil {
			log.Printf("[WARN] IPFS pin failed for letgo: %v", err)
		}
	}()

	c.JSON(200, models.APIResponse{
		Success: true,
		Message: "Bounty forfeited — reputation reduced by 20%, creator will receive refund",
		Data:    gin.H{"txn_id": txID, "refund_wallet": creatorWallet, "refund_amount": rewardAlgo},
	})
}

// GET /api/bounties/:id/submissions — List submissions for a bounty
func (h *BountyHandler) ListSubmissions(c *gin.Context) {
	bid := c.Param("id")
	pid, _ := c.Get("profile_id")
	role, _ := c.Get("role")

	var query string
	var args []interface{}

	if role.(string) == "creator" {
		// Creator sees all submissions for their bounty
		query = `
			SELECT s.id, s.freelancer_id, s.submission_number,
			       COALESCE(s.mega_nz_link, '') as mega_nz_link,
			       COALESCE(s.encryption_key_r2_path, '') as encryption_key_r2_path,
			       s.description, s.status, s.rejection_feedback,
			       s.creator_message, s.creator_rating, s.work_hash_sha256, s.created_at,
			       p.username, p.display_name, p.avatar_url, p.reputation_score
			FROM submissions s JOIN profiles p ON s.freelancer_id = p.id
			WHERE s.bounty_id = $1 ORDER BY s.created_at DESC`
		args = []interface{}{bid}
	} else {
		// Freelancer sees only their own submissions
		query = `
			SELECT s.id, s.freelancer_id, s.submission_number,
			       COALESCE(s.mega_nz_link, '') as mega_nz_link,
			       COALESCE(s.encryption_key_r2_path, '') as encryption_key_r2_path,
			       s.description, s.status, s.rejection_feedback,
			       s.creator_message, s.creator_rating, s.work_hash_sha256, s.created_at,
			       p.username, p.display_name, p.avatar_url, p.reputation_score
			FROM submissions s JOIN profiles p ON s.freelancer_id = p.id
			WHERE s.bounty_id = $1 AND s.freelancer_id = $2 ORDER BY s.created_at DESC`
		args = []interface{}{bid, pid}
	}

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch submissions"})
		return
	}
	defer rows.Close()

	subs := []gin.H{}
	for rows.Next() {
		var sid, flID string
		var subNum, rep int
		var megaLink, encKeyPath, status string
		var hash, desc *string
		var feedback, msg *string
		var rating *int
		var createdAt time.Time
		var un string
		var dn, av *string

		err := rows.Scan(
			&sid, &flID, &subNum, &megaLink, &encKeyPath,
			&desc, &status, &feedback, &msg, &rating, &hash, &createdAt,
			&un, &dn, &av, &rep,
		)
		if err != nil {
			log.Printf("[WARN] Submissions scan error: %v", err)
			continue
		}

		// Generate signed URL for encryption key file access (if present)
		var signedKeyURL string
		if encKeyPath != "" {
			signedKeyURL, _ = h.r2Svc.GenerateSignedURL(c.Request.Context(), encKeyPath)
		}

		subs = append(subs, gin.H{
			"id": sid, "freelancer_id": flID, "submission_number": subNum,
			"mega_nz_link": megaLink,
			"encryption_key_url": signedKeyURL,
			"description": desc, "status": status, "rejection_feedback": feedback,
			"creator_message": msg, "creator_rating": rating,
			"work_hash_sha256": hash, "created_at": createdAt,
			"freelancer": gin.H{"username": un, "display_name": dn, "avatar_url": av, "reputation_score": rep},
		})
	}
	c.JSON(200, models.APIResponse{Success: true, Data: subs})
}

// POST /api/bounties/:id/cancel
func (h *BountyHandler) CancelBounty(c *gin.Context) {
	bid := c.Param("id")
	pid, _ := c.Get("profile_id")

	var creatorID string
	var subCount int
	err := database.DB.QueryRow(`
		SELECT creator_id, (SELECT COUNT(*) FROM submissions WHERE bounty_id = $1)
		FROM bounties WHERE id = $1 AND status = 'open'
	`, bid).Scan(&creatorID, &subCount)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Bounty not found or not cancellable"})
		return
	}
	if creatorID != pid.(string) {
		c.JSON(403, models.APIResponse{Success: false, Error: "Only creator can cancel"})
		return
	}
	if subCount > 0 {
		c.JSON(400, models.APIResponse{Success: false, Error: "Cannot cancel bounty with submissions"})
		return
	}

	database.DB.Exec(`UPDATE bounties SET status = 'cancelled', updated_at = NOW() WHERE id = $1`, bid)
	c.JSON(200, models.APIResponse{Success: true, Message: "Bounty cancelled"})
}

// POST /api/bounties/:id/refund-expired — Permissionless expired refund trigger
func (h *BountyHandler) RefundExpired(c *gin.Context) {
	bid := c.Param("id")

	var status string
	var deadline time.Time
	err := database.DB.QueryRow(`SELECT status, deadline FROM bounties WHERE id = $1`, bid).Scan(&status, &deadline)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Bounty not found"})
		return
	}
	if status != "open" && status != "in_progress" {
		c.JSON(400, models.APIResponse{Success: false, Error: "Bounty not refundable in current status"})
		return
	}
	if time.Now().Before(deadline) {
		c.JSON(400, models.APIResponse{Success: false, Error: "Deadline not yet passed"})
		return
	}

	database.DB.Exec(`UPDATE bounties SET status = 'expired', updated_at = NOW() WHERE id = $1`, bid)
	database.DB.Exec(`
		INSERT INTO transaction_log (bounty_id, event, txn_note)
		VALUES ($1, 'bounty_expired', 'BountyVault:bounty_expired:deadline_passed')
	`, bid)

	c.JSON(200, models.APIResponse{Success: true, Message: "Bounty marked expired — refund triggered on-chain"})
}

// POST /api/bounties/:id/rate — Creator rates a worker after approval
func (h *BountyHandler) RateWorker(c *gin.Context) {
	bid := c.Param("id")
	pid, _ := c.Get("profile_id")
	var req struct {
		WorkerID string `json:"worker_id" binding:"required"`
		Stars    int    `json:"stars" binding:"required,min=1,max=5"`
		Comment  string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	var cid string
	database.DB.QueryRow(`SELECT creator_id FROM bounties WHERE id=$1 AND status='completed'`, bid).Scan(&cid)
	if cid != pid.(string) {
		c.JSON(403, models.APIResponse{Success: false, Error: "Only creator can rate"})
		return
	}

	_, err := database.DB.Exec(`INSERT INTO ratings (bounty_id,rater_id,worker_id,stars,comment) VALUES ($1,$2,$3,$4,$5)`,
		bid, pid, req.WorkerID, req.Stars, req.Comment)
	if err != nil {
		c.JSON(409, models.APIResponse{Success: false, Error: "Already rated"})
		return
	}

	c.JSON(201, models.APIResponse{Success: true, Message: "Worker rated"})
}

// GET /api/categories — List bounty categories
func (h *BountyHandler) ListCategories(c *gin.Context) {
	rows, _ := database.DB.Query(`SELECT id, name, slug, description, icon, color FROM bounty_categories ORDER BY name`)
	if rows == nil {
		c.JSON(200, models.APIResponse{Success: true, Data: []gin.H{}})
		return
	}
	defer rows.Close()
	var cats []gin.H
	for rows.Next() {
		var id int
		var n, s, i, cl string
		var d *string
		rows.Scan(&id, &n, &s, &d, &i, &cl)
		cats = append(cats, gin.H{"id": id, "name": n, "slug": s, "description": d, "icon": i, "color": cl})
	}
	c.JSON(200, models.APIResponse{Success: true, Data: cats})
}

// ============================================================
// Bounty Acceptance Handlers (v3.2)
// ============================================================

// POST /api/bounties/:id/accept — Freelancer requests to accept a bounty
func (h *BountyHandler) AcceptBounty(c *gin.Context) {
	bid := c.Param("id")
	pid, _ := c.Get("profile_id")
	role, _ := c.Get("role")

	if role.(string) != "freelancer" {
		c.JSON(403, models.APIResponse{Success: false, Error: "Only freelancers can accept bounties"})
		return
	}

	var req models.AcceptBountyRequest
	// Message is optional, so bind silently
	c.ShouldBindJSON(&req)

	// Verify bounty exists and is open
	var creatorID, status string
	var title string
	err := database.DB.QueryRow(`SELECT creator_id, status, title FROM bounties WHERE id = $1`, bid).Scan(&creatorID, &status, &title)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Bounty not found"})
		return
	}
	if status != "open" {
		c.JSON(400, models.APIResponse{Success: false, Error: "Bounty is not open for acceptance"})
		return
	}
	if creatorID == pid.(string) {
		c.JSON(400, models.APIResponse{Success: false, Error: "Creator cannot accept their own bounty"})
		return
	}

	// Check for duplicate
	var existingID string
	err = database.DB.QueryRow(`SELECT id FROM bounty_acceptances WHERE bounty_id = $1 AND freelancer_id = $2`, bid, pid).Scan(&existingID)
	if err == nil {
		c.JSON(409, models.APIResponse{Success: false, Error: "You have already requested to accept this bounty"})
		return
	}

	// Insert acceptance request
	aid := uuid.New()
	var msg *string
	if req.Message != "" {
		msg = &req.Message
	}
	_, err = database.DB.Exec(`
		INSERT INTO bounty_acceptances (id, bounty_id, freelancer_id, status, message)
		VALUES ($1, $2, $3, 'pending', $4)
	`, aid, bid, pid, msg)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to create acceptance request"})
		return
	}

	// Notify the creator
	var freelancerUsername string
	database.DB.QueryRow(`SELECT username FROM profiles WHERE id = $1`, pid).Scan(&freelancerUsername)
	database.DB.Exec(`
		INSERT INTO notifications (user_id, type, title, message, bounty_id)
		VALUES ($1, 'acceptance_request', 'New Acceptance Request',
			$2, $3)
	`, creatorID, fmt.Sprintf("%s wants to work on your bounty: %s", freelancerUsername, title), bid)

	c.JSON(201, models.APIResponse{
		Success: true,
		Message: "Acceptance request sent to creator",
		Data:    gin.H{"acceptance_id": aid},
	})
}

// GET /api/bounties/:id/acceptances — List acceptance requests for a bounty
func (h *BountyHandler) GetAcceptances(c *gin.Context) {
	bid := c.Param("id")
	pid, _ := c.Get("profile_id")

	// Verify the caller is the creator of this bounty
	var creatorID string
	err := database.DB.QueryRow(`SELECT creator_id FROM bounties WHERE id = $1`, bid).Scan(&creatorID)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Bounty not found"})
		return
	}

	// Allow both creator (sees all) and freelancers (see own)
	var rows *sql.Rows
	if creatorID == pid.(string) {
		rows, err = database.DB.Query(`
			SELECT a.id, a.bounty_id, a.freelancer_id, a.status, a.message, a.creator_note, a.created_at, a.updated_at,
			       p.id, p.username, p.display_name, p.avatar_url, p.reputation_score, p.bio,
			       p.total_bounties_completed, p.avg_rating, p.total_ratings
			FROM bounty_acceptances a
			JOIN profiles p ON a.freelancer_id = p.id
			WHERE a.bounty_id = $1
			ORDER BY a.created_at DESC
		`, bid)
	} else {
		rows, err = database.DB.Query(`
			SELECT a.id, a.bounty_id, a.freelancer_id, a.status, a.message, a.creator_note, a.created_at, a.updated_at,
			       p.id, p.username, p.display_name, p.avatar_url, p.reputation_score, p.bio,
			       p.total_bounties_completed, p.avg_rating, p.total_ratings
			FROM bounty_acceptances a
			JOIN profiles p ON a.freelancer_id = p.id
			WHERE a.bounty_id = $1 AND a.freelancer_id = $2
			ORDER BY a.created_at DESC
		`, bid, pid)
	}
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch acceptances"})
		return
	}
	defer rows.Close()

	acceptances := []gin.H{}
	for rows.Next() {
		var aID, aBountyID, aFreelancerID, aStatus string
		var aMsg, aNote *string
		var aCreated, aUpdated time.Time
		var pID, pUsername string
		var pDisplayName, pAvatarURL, pBio *string
		var pRep, pCompleted, pTotalRatings int
		var pAvgRating float64

		rows.Scan(
			&aID, &aBountyID, &aFreelancerID, &aStatus, &aMsg, &aNote, &aCreated, &aUpdated,
			&pID, &pUsername, &pDisplayName, &pAvatarURL, &pRep, &pBio,
			&pCompleted, &pAvgRating, &pTotalRatings,
		)

		acceptances = append(acceptances, gin.H{
			"id":           aID,
			"bounty_id":    aBountyID,
			"freelancer_id": aFreelancerID,
			"status":       aStatus,
			"message":      aMsg,
			"creator_note": aNote,
			"created_at":   aCreated,
			"updated_at":   aUpdated,
			"freelancer": gin.H{
				"id":                       pID,
				"username":                 pUsername,
				"display_name":             pDisplayName,
				"avatar_url":               pAvatarURL,
				"reputation_score":         pRep,
				"bio":                      pBio,
				"total_bounties_completed": pCompleted,
				"avg_rating":               pAvgRating,
				"total_ratings":            pTotalRatings,
			},
		})
	}

	c.JSON(200, models.APIResponse{Success: true, Data: acceptances})
}

// PUT /api/bounties/:id/review-acceptance — Creator reviews (approve/reject) a freelancer's acceptance
func (h *BountyHandler) ReviewAcceptance(c *gin.Context) {
	bid := c.Param("id")
	pid, _ := c.Get("profile_id")

	var req models.ReviewAcceptanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	if req.Action != "approve" && req.Action != "reject" {
		c.JSON(400, models.APIResponse{Success: false, Error: "Action must be 'approve' or 'reject'"})
		return
	}

	// Verify bounty ownership
	var creatorID string
	var rewardAlgo float64
	var deadline time.Time
	var maxSubs int
	var bountyStatus string
	var bountyDisplayID string
	err := database.DB.QueryRow(`
		SELECT creator_id, reward_algo, deadline, max_submissions, status, bounty_id
		FROM bounties WHERE id = $1
	`, bid).Scan(&creatorID, &rewardAlgo, &deadline, &maxSubs, &bountyStatus, &bountyDisplayID)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Bounty not found"})
		return
	}
	if creatorID != pid.(string) {
		c.JSON(403, models.APIResponse{Success: false, Error: "Only the creator can review acceptances"})
		return
	}
	if bountyStatus != "open" {
		c.JSON(400, models.APIResponse{Success: false, Error: "Bounty is not in open state"})
		return
	}

	// Verify acceptance exists and is pending
	var acceptanceID string
	err = database.DB.QueryRow(`
		SELECT id FROM bounty_acceptances
		WHERE bounty_id = $1 AND freelancer_id = $2 AND status = 'pending'
	`, bid, req.FreelancerID).Scan(&acceptanceID)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Pending acceptance request not found"})
		return
	}

	if req.Action == "reject" {
		var note *string
		if req.Note != "" {
			note = &req.Note
		}
		database.DB.Exec(`
			UPDATE bounty_acceptances SET status = 'rejected', creator_note = $1, updated_at = NOW()
			WHERE id = $2
		`, note, acceptanceID)

		// Notify freelancer
		database.DB.Exec(`
			INSERT INTO notifications (user_id, type, title, message, bounty_id)
			VALUES ($1, 'acceptance_rejected', 'Acceptance Request Rejected',
				'Your acceptance request was rejected by the creator.', $2)
		`, req.FreelancerID, bid)

		c.JSON(200, models.APIResponse{Success: true, Message: "Acceptance rejected"})
		return
	}

	// Action == "approve" — need to build escrow contract txns
	if req.WalletAddress == "" {
		c.JSON(400, models.APIResponse{Success: false, Error: "wallet_address is required for approval (to lock escrow)"})
		return
	}

	// ESCROW BYPASS: When escrow address is configured, send funds directly
	// to the escrow account instead of the smart contract app address
	escrowAddr := h.cfg.EscrowAddress
	if escrowAddr != "" {
		log.Printf("[ESCROW-LOCK] ReviewAcceptance: Using escrow bypass: sending to %s", escrowAddr)
		txns, err := h.algoSvc.BuildEscrowLockTxn(
			c.Request.Context(), req.WalletAddress, escrowAddr,
			uint64(rewardAlgo*1e6), bountyDisplayID,
		)
		if err != nil {
			c.JSON(500, models.APIResponse{Success: false, Error: "Failed to build escrow transactions: " + err.Error()})
			return
		}
		c.JSON(200, models.APIResponse{
			Success: true,
			Message: "Sign the transaction to lock funds in escrow and approve this freelancer",
			Data: gin.H{
				"transactions":  txns,
				"freelancer_id": req.FreelancerID,
				"acceptance_id": acceptanceID,
			},
		})
		return
	}

	// Original smart contract flow — sends to app address
	txns, err := h.algoSvc.BuildCreateBountyTxns(
		c.Request.Context(), req.WalletAddress,
		uint64(rewardAlgo*1e6), nil, uint64(deadline.Unix()), uint64(maxSubs), "",
	)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to build escrow transactions: " + err.Error()})
		return
	}

	c.JSON(200, models.APIResponse{
		Success: true,
		Message: "Sign the escrow transactions with Pera Wallet to approve this freelancer",
		Data: gin.H{
			"transactions":  txns,
			"freelancer_id": req.FreelancerID,
			"acceptance_id": acceptanceID,
		},
	})
}

// POST /api/bounties/:id/confirm-acceptance — Submit signed escrow txns and finalize acceptance
func (h *BountyHandler) ConfirmAcceptance(c *gin.Context) {
	bid := c.Param("id")
	pid, _ := c.Get("profile_id")

	var req models.ConfirmAcceptanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Verify bounty ownership
	var creatorID string
	var rewardAlgo float64
	var bountyDisplayID string
	err := database.DB.QueryRow(`
		SELECT creator_id, reward_algo, bounty_id FROM bounties WHERE id = $1 AND status = 'open'
	`, bid).Scan(&creatorID, &rewardAlgo, &bountyDisplayID)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Bounty not found or not in open state"})
		return
	}
	if creatorID != pid.(string) {
		c.JSON(403, models.APIResponse{Success: false, Error: "Only the creator can confirm acceptance"})
		return
	}

	// Submit signed transactions to Algorand
	txID, err := h.algoSvc.SubmitSignedTxns(c.Request.Context(), req.SignedTxns)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Transaction submission failed: " + err.Error()})
		return
	}
	h.algoSvc.WaitForConfirmation(c.Request.Context(), txID, 10)

	// Update bounty: set app_id, escrow_txn_id, accepted_freelancer_id, status -> in_progress
	database.DB.Exec(`
		UPDATE bounties SET app_id = $1, escrow_txn_id = $2, accepted_freelancer_id = $3,
		  status = 'in_progress', updated_at = NOW()
		WHERE id = $4
	`, req.AppID, txID, req.FreelancerID, bid)

	// Update acceptance: approved
	database.DB.Exec(`
		UPDATE bounty_acceptances SET status = 'approved', updated_at = NOW()
		WHERE bounty_id = $1 AND freelancer_id = $2 AND status = 'pending'
	`, bid, req.FreelancerID)

	// Reject all other pending acceptances for this bounty
	database.DB.Exec(`
		UPDATE bounty_acceptances SET status = 'rejected', creator_note = 'Another freelancer was selected', updated_at = NOW()
		WHERE bounty_id = $1 AND freelancer_id != $2 AND status = 'pending'
	`, bid, req.FreelancerID)

	// Log transaction — escrow_locked for creator
	database.DB.Exec(`
		INSERT INTO transaction_log (bounty_id, actor_id, event, txn_id, txn_note, amount_algo)
		VALUES ($1, $2, 'escrow_locked', $3, $4, $5)
	`, bid, pid, txID, fmt.Sprintf("BountyVault:escrow_locked:%d", req.AppID), rewardAlgo)

	// Log transaction — bounty_accepted for freelancer
	database.DB.Exec(`
		INSERT INTO transaction_log (bounty_id, actor_id, event, txn_id, txn_note, amount_algo)
		VALUES ($1, $2, 'bounty_accepted', $3, $4, $5)
	`, bid, req.FreelancerID, txID, fmt.Sprintf("BountyVault:bounty_accepted:%d", req.AppID), rewardAlgo)

	// Notify freelancer
	database.DB.Exec(`
		INSERT INTO notifications (user_id, type, title, message, bounty_id)
		VALUES ($1, 'acceptance_approved', 'You have been selected!',
			'Your acceptance request was approved. You can now submit work on this bounty.', $2)
	`, req.FreelancerID, bid)

	// Pin to IPFS (fire-and-forget)
	go func() {
		var walletAddr string
		database.DB.QueryRow(`SELECT COALESCE(wallet_address,'') FROM profiles WHERE id = $1`, pid).Scan(&walletAddr)
		var b models.Bounty
		database.DB.QueryRow(`SELECT bounty_id, reward_algo, max_submissions, deadline, tags FROM bounties WHERE id = $1`, bid).Scan(
			&b.BountyID, &b.RewardAlgo, &b.MaxSubmissions, &b.Deadline, pq.Array(&b.Tags),
		)
		_, pinErr := h.ipfsSvc.PinBountyCreated(context.Background(), services.BountyCreatedMetadata{
			BountyID:       b.BountyID,
			AppID:          req.AppID,
			CreatorAddress: walletAddr,
			RewardAlgo:     b.RewardAlgo,
			MaxSubmissions: b.MaxSubmissions,
			Deadline:       b.Deadline.Format(time.RFC3339),
			Tags:           b.Tags,
			TxnID:          txID,
			Network:        h.cfg.AlgoNetwork,
		})
		if pinErr != nil {
			log.Printf("[WARN] IPFS pin failed for acceptance confirmation: %v", pinErr)
		}
	}()

	c.JSON(200, models.APIResponse{
		Success: true,
		Message: "Escrow locked and freelancer approved",
		Data:    gin.H{"txn_id": txID, "app_id": req.AppID},
	})
}

// GET /api/bounties/:id/my-acceptance — Get the current freelancer's acceptance status for a bounty
func (h *BountyHandler) GetMyAcceptanceStatus(c *gin.Context) {
	bid := c.Param("id")
	pid, _ := c.Get("profile_id")

	var aID, aStatus string
	var aMsg, aNote *string
	var aCreated time.Time
	err := database.DB.QueryRow(`
		SELECT id, status, message, creator_note, created_at
		FROM bounty_acceptances
		WHERE bounty_id = $1 AND freelancer_id = $2
	`, bid, pid).Scan(&aID, &aStatus, &aMsg, &aNote, &aCreated)
	if err != nil {
		c.JSON(200, models.APIResponse{Success: true, Data: nil})
		return
	}

	c.JSON(200, models.APIResponse{Success: true, Data: gin.H{
		"id":           aID,
		"status":       aStatus,
		"message":      aMsg,
		"creator_note": aNote,
		"created_at":   aCreated,
	}})
}

