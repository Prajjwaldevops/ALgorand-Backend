package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"bountyvault/internal/config"
	"bountyvault/internal/database"
	"bountyvault/internal/models"
	"bountyvault/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// DAOHandler handles DAO voting endpoints
// v3.1 dispute resolution + v3.6 voting compliance
// Aligned with BountyEscrow smart contract (bounty_escrow.py)
type DAOHandler struct {
	cfg     *config.Config
	algoSvc *services.AlgorandService
	ipfsSvc *services.IPFSService
}

func NewDAOHandler(cfg *config.Config, algoSvc *services.AlgorandService, ipfsSvc *services.IPFSService) *DAOHandler {
	return &DAOHandler{cfg: cfg, algoSvc: algoSvc, ipfsSvc: ipfsSvc}
}

// GET /api/dao/disputes — List all active disputes with vote tallies
func (h *DAOHandler) ListActiveDisputes(c *gin.Context) {
	rows, err := database.DB.Query(`
		SELECT d.id, COALESCE(d.dispute_id, d.id::text), d.bounty_id,
		       COALESCE(d.freelancer_description, d.reason, ''), d.status::text,
		       COALESCE(d.voting_deadline, d.dao_vote_deadline, NOW()),
		       COALESCE(d.votes_creator, 0), COALESCE(d.votes_freelancer, 0), d.created_at,
		       b.title, b.reward_algo, b.deadline,
		       COALESCE(pf.username, 'Unknown') AS freelancer_name,
		       COALESCE(pc.username, 'Unknown') AS creator_name
		FROM disputes d
		JOIN bounties b ON d.bounty_id = b.id
		LEFT JOIN profiles pf ON COALESCE(d.freelancer_id, d.initiated_by) = pf.id
		LEFT JOIN profiles pc ON d.creator_id = pc.id
		WHERE d.status::text IN ('open', 'voting')
		ORDER BY d.created_at DESC
	`)
	if err != nil {
		log.Printf("[ERROR] ListActiveDisputes query failed: %v", err)
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch disputes: " + err.Error()})
		return
	}
	defer rows.Close()

	var disputes []gin.H
	for rows.Next() {
		var did uuid.UUID
		var disputeDisplayID, status, freelancerName, creatorName, bTitle string
		var bid uuid.UUID
		var description string
		var voteDeadline, createdAt time.Time
		var votesCreator, votesFreelancer int
		var bDeadline time.Time
		var reward float64

		rows.Scan(&did, &disputeDisplayID, &bid, &description, &status,
			&voteDeadline, &votesCreator, &votesFreelancer, &createdAt,
			&bTitle, &reward, &bDeadline,
			&freelancerName, &creatorName)

		disputes = append(disputes, gin.H{
			"id": did, "dispute_id": disputeDisplayID, "bounty_id": bid,
			"description": description, "status": status,
			"voting_deadline": voteDeadline, "created_at": createdAt,
			"freelancer_name": freelancerName, "creator_name": creatorName,
			"bounty": gin.H{"title": bTitle, "reward_algo": reward, "deadline": bDeadline},
			"votes": gin.H{
				"creator":    votesCreator,
				"freelancer": votesFreelancer,
				"total":      votesCreator + votesFreelancer,
			},
			"voting_active": time.Now().Before(voteDeadline),
		})
	}

	c.JSON(200, models.APIResponse{Success: true, Data: disputes})
}

// GET /api/dao/disputes/:id — Get full dispute detail with conversation history
func (h *DAOHandler) GetDisputeDetail(c *gin.Context) {
	disputeID := c.Param("id")

	var did uuid.UUID
	var disputeDisplayID, status, freelancerDesc string
	var bid, freelancerProfileID uuid.UUID
	var creatorProfileID uuid.UUID
	var voteDeadline, createdAt time.Time
	var votesCreator, votesFreelancer int
	var resolvedAt time.Time
	var resolutionTxnID, ipfsDisputeCID string
	var submissionHistoryJSON []byte
	var bTitle string
	var reward float64
	var bDeadline time.Time
	var freelancerName, creatorName string

	err := database.DB.QueryRow(`
		SELECT d.id, COALESCE(d.dispute_id, d.id::text), d.bounty_id,
		       COALESCE(d.freelancer_id, d.initiated_by) AS freelancer_id,
		       COALESCE(d.creator_id, '00000000-0000-0000-0000-000000000000'::uuid),
		       COALESCE(d.freelancer_description, d.reason, '') AS freelancer_description,
		       COALESCE(d.submission_history, '[]'::jsonb) AS submission_history,
		       d.status::text,
		       COALESCE(d.votes_creator, 0), COALESCE(d.votes_freelancer, 0),
		       COALESCE(d.voting_deadline, d.dao_vote_deadline, NOW()),
		       COALESCE(d.resolved_at, '1970-01-01 00:00:00Z'::timestamptz),
		       COALESCE(d.resolution_txn_id, ''),
		       COALESCE(d.ipfs_dispute_cid, ''),
		       d.created_at,
		       b.title, b.reward_algo, b.deadline,
		       COALESCE(pf.username, 'Unknown') AS freelancer_name,
		       COALESCE(pc.username, 'Unknown') AS creator_name
		FROM disputes d
		JOIN bounties b ON d.bounty_id = b.id
		LEFT JOIN profiles pf ON COALESCE(d.freelancer_id, d.initiated_by) = pf.id
		LEFT JOIN profiles pc ON d.creator_id = pc.id
		WHERE CAST(d.id AS TEXT) = $1 OR d.dispute_id = $1
	`, disputeID).Scan(
		&did, &disputeDisplayID, &bid, &freelancerProfileID, &creatorProfileID,
		&freelancerDesc, &submissionHistoryJSON, &status,
		&votesCreator, &votesFreelancer, &voteDeadline,
		&resolvedAt, &resolutionTxnID, &ipfsDisputeCID, &createdAt,
		&bTitle, &reward, &bDeadline,
		&freelancerName, &creatorName,
	)

	if err == sql.ErrNoRows {
		c.JSON(404, models.APIResponse{Success: false, Error: "Dispute not found"})
		return
	}
	if err != nil {
		log.Printf("[ERROR] GetDisputeDetail query failed for id=%s: %v", disputeID, err)
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch dispute: " + err.Error()})
		return
	}

	var submissionHistory []gin.H
	if len(submissionHistoryJSON) > 0 {
		var rawHistory []map[string]interface{}
		if err := json.Unmarshal(submissionHistoryJSON, &rawHistory); err == nil {
			for _, entry := range rawHistory {
				submissionHistory = append(submissionHistory, gin.H(entry))
			}
		}
	}

	var conversationEntries []gin.H
	rows, err := database.DB.Query(`
		SELECT s.submission_number, s.description, s.status,
		       COALESCE(s.rejection_feedback, '') as rejection_feedback,
		       COALESCE(s.work_hash_sha256, '') as work_hash,
		       COALESCE(s.mega_nz_link, '') as mega_nz_link,
		       s.created_at, s.resolved_at
		FROM submissions s
		WHERE s.bounty_id = $1 AND s.freelancer_id = $2
		ORDER BY s.submission_number ASC
	`, bid, freelancerProfileID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var subNum int
			var desc, subStatus, rejFeedback, workHash, megaLink string
			var subCreatedAt time.Time
			var subResolvedAt *time.Time
			rows.Scan(&subNum, &desc, &subStatus, &rejFeedback, &workHash, &megaLink, &subCreatedAt, &subResolvedAt)

			entry := gin.H{
				"submission_number":  subNum,
				"description":        desc,
				"status":             subStatus,
				"rejection_feedback": rejFeedback,
				"work_hash":          workHash,
				"mega_nz_link":       megaLink,
				"submitted_at":       subCreatedAt,
			}
			if subResolvedAt != nil {
				entry["reviewed_at"] = *subResolvedAt
			}
			conversationEntries = append(conversationEntries, entry)
		}
	}

	c.JSON(200, models.APIResponse{Success: true, Data: gin.H{
		"id": did, "dispute_id": disputeDisplayID, "bounty_id": bid,
		"freelancer_id": freelancerProfileID, "creator_id": creatorProfileID,
		"freelancer_description": freelancerDesc,
		"submission_history":     submissionHistory,
		"conversation":           conversationEntries,
		"status": status, "votes_creator": votesCreator, "votes_freelancer": votesFreelancer,
		"voting_deadline": voteDeadline, "resolved_at": resolvedAt,
		"resolution_txn_id": resolutionTxnID, "ipfs_dispute_cid": ipfsDisputeCID,
		"created_at": createdAt, "freelancer_name": freelancerName, "creator_name": creatorName,
		"bounty":       gin.H{"title": bTitle, "reward_algo": reward, "deadline": bDeadline},
		"votes":        gin.H{"creator": votesCreator, "freelancer": votesFreelancer, "total": votesCreator + votesFreelancer},
		"voting_active": time.Now().Before(voteDeadline) && status == "open",
	}})
}

// POST /api/dao/disputes/:id/build-vote-txn — Build unsigned grouped txn for DAO vote
// Smart contract aligned: builds Payment(0.001 ALGO → escrow) + AppCall(cast_dao_vote)
func (h *DAOHandler) BuildVoteTxn(c *gin.Context) {
	pid, _ := c.Get("profile_id")
	var req struct {
		WalletAddress string `json:"wallet_address" binding:"required"`
		Vote          string `json:"vote" binding:"required"` // "creator" or "freelancer"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: "wallet_address and vote are required"})
		return
	}

	disputeID := c.Param("id")

	// Verify dispute exists and voting is open
	var status string
	var voteDeadline time.Time
	var bountyCreatorID, freelancerID, bountyID string
	var appID *int64
	err := database.DB.QueryRow(`
		SELECT d.status::text, COALESCE(d.voting_deadline, d.dao_vote_deadline, NOW()),
		       COALESCE(d.creator_id::text, ''), COALESCE(d.freelancer_id::text, COALESCE(d.initiated_by::text, '')), d.bounty_id
		FROM disputes d WHERE d.id = $1
	`, disputeID).Scan(&status, &voteDeadline, &bountyCreatorID, &freelancerID, &bountyID)

	if err == sql.ErrNoRows {
		c.JSON(404, models.APIResponse{Success: false, Error: "Dispute not found"})
		return
	}
	if err != nil {
		log.Printf("[ERROR] BuildVoteTxn query failed: %v", err)
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch dispute"})
		return
	}
	if status != "open" && status != "voting" {
		c.JSON(400, models.APIResponse{Success: false, Error: "Dispute voting is closed"})
		return
	}
	if time.Now().After(voteDeadline) {
		c.JSON(400, models.APIResponse{Success: false, Error: "Voting period has expired"})
		return
	}
	if bountyCreatorID == pid.(string) {
		c.JSON(403, models.APIResponse{Success: false, Error: "Bounty creator cannot vote"})
		return
	}
	if freelancerID == pid.(string) {
		c.JSON(403, models.APIResponse{Success: false, Error: "Disputing freelancer cannot vote"})
		return
	}

	// Check if already voted
	var existingVote int
	database.DB.QueryRow(`SELECT COUNT(*) FROM dao_votes WHERE dispute_id = $1 AND voter_id = $2`, disputeID, pid).Scan(&existingVote)
	if existingVote > 0 {
		c.JSON(409, models.APIResponse{Success: false, Error: "You have already voted on this dispute"})
		return
	}

	if h.algoSvc == nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Algorand service unavailable"})
		return
	}

	// Get the bounty's per-bounty app_id for the smart contract call
	database.DB.QueryRow(`SELECT app_id FROM bounties WHERE id = $1`, bountyID).Scan(&appID)
	var contractAppID uint64
	if appID != nil && *appID > 0 {
		contractAppID = uint64(*appID)
	}

	// Map vote string to smart contract enum: 1=creator, 2=freelancer
	var voteFor uint64
	if req.Vote == "creator" {
		voteFor = 1 // VOTE_CREATOR in contract
	} else {
		voteFor = 2 // VOTE_FREELANCER in contract
	}

	// Escrow address = the contract app address or configured escrow
	escrowAddr := h.cfg.EscrowAddress
	if escrowAddr == "" {
		escrowAddr = "ZPEOMV6AYCWGTPKAOQ4D33UR3UWJTBK7O3G6DAW6RBTF5BQZTZLMWAQNBM"
	}

	// Build grouped txn: Payment(0.001 ALGO → escrow) + AppCall(cast_dao_vote)
	txnResult, err := h.algoSvc.BuildVotePaymentTxn(
		c.Request.Context(), req.WalletAddress, escrowAddr, voteFor, contractAppID,
	)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to build vote transaction: " + err.Error()})
		return
	}

	c.JSON(200, models.APIResponse{
		Success: true,
		Message: "Sign both transactions: 0.001 ALGO gas fee + DAO vote smart contract call",
		Data: gin.H{
			"transactions":   txnResult.Transactions,
			"group_id":       txnResult.GroupID,
			"escrow_address": escrowAddr,
			"gas_fee_algo":   0.001,
			"vote_for":       voteFor,
			"app_id":         contractAppID,
		},
	})
}

// POST /api/dao/disputes/:id/vote — Submit signed vote txns and record vote
// Smart contract aligned: expects signed group (Payment + cast_dao_vote app call)
func (h *DAOHandler) CastVote(c *gin.Context) {
	pid, _ := c.Get("profile_id")
	var req models.CastDAOVoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: "Invalid request: " + err.Error()})
		return
	}

	disputeID := c.Param("id")

	var status string
	var voteDeadline time.Time
	var bountyCreatorID, freelancerID, bountyID string
	var disputeDisplayID string
	err := database.DB.QueryRow(`
		SELECT d.status::text, COALESCE(d.voting_deadline, d.dao_vote_deadline, NOW()),
		       COALESCE(d.creator_id::text, ''), COALESCE(d.freelancer_id::text, COALESCE(d.initiated_by::text, '')),
		       d.bounty_id, COALESCE(d.dispute_id, d.id::text)
		FROM disputes d WHERE d.id = $1
	`, disputeID).Scan(&status, &voteDeadline, &bountyCreatorID, &freelancerID, &bountyID, &disputeDisplayID)

	if err == sql.ErrNoRows {
		c.JSON(404, models.APIResponse{Success: false, Error: "Dispute not found"})
		return
	}
	if err != nil {
		log.Printf("[ERROR] CastVote dispute query failed: %v", err)
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch dispute: " + err.Error()})
		return
	}
	if status != "open" && status != "voting" {
		c.JSON(400, models.APIResponse{Success: false, Error: "Dispute voting is closed"})
		return
	}
	if time.Now().After(voteDeadline) {
		c.JSON(400, models.APIResponse{Success: false, Error: "Voting period has expired"})
		return
	}
	if bountyCreatorID == pid.(string) {
		c.JSON(403, models.APIResponse{Success: false, Error: "Bounty creator cannot vote"})
		return
	}
	if freelancerID == pid.(string) {
		c.JSON(403, models.APIResponse{Success: false, Error: "Disputing freelancer cannot vote"})
		return
	}

	// Submit the signed grouped transaction (Payment + cast_dao_vote app call)
	var txID string
	if len(req.SignedTxns) > 0 && req.SignedTxns[0] != "placeholder-signed-txn" && h.algoSvc != nil {
		var err error
		txID, err = h.algoSvc.SubmitSignedTxns(c.Request.Context(), req.SignedTxns)
		if err != nil {
			c.JSON(500, models.APIResponse{Success: false, Error: "Blockchain transaction failed: " + err.Error()})
			return
		}
		h.algoSvc.WaitForConfirmation(c.Request.Context(), txID, 10)
	} else {
		txID = fmt.Sprintf("mock_vote_%d", time.Now().UnixMilli())
	}

	// Insert vote (UNIQUE constraint prevents double voting)
	var walletAddr *string
	database.DB.QueryRow(`SELECT wallet_address FROM profiles WHERE id=$1`, pid).Scan(&walletAddr)

	vid := uuid.New()
	_, err = database.DB.Exec(`
		INSERT INTO dao_votes (id, dispute_id, voter_id, vote, vote_txn_id)
		VALUES ($1, $2, $3, $4::dao_vote_choice, $5)
	`, vid, disputeID, pid, req.Vote, txID)
	if err != nil {
		log.Printf("[ERROR] Failed to insert DAO vote: %v", err)
		c.JSON(409, models.APIResponse{Success: false, Error: "Database error or you already voted: " + err.Error()})
		return
	}

	// Update tally on dispute record
	if req.Vote == "creator" {
		database.DB.Exec(`UPDATE disputes SET votes_creator = votes_creator + 1 WHERE id = $1`, disputeID)
	} else {
		database.DB.Exec(`UPDATE disputes SET votes_freelancer = votes_freelancer + 1 WHERE id = $1`, disputeID)
	}

	var vc, vf int
	database.DB.QueryRow(`SELECT COALESCE(votes_creator,0), COALESCE(votes_freelancer,0) FROM disputes WHERE id = $1`, disputeID).Scan(&vc, &vf)

	// Update voting compliance: clear ban, set last vote time
	database.DB.Exec(`UPDATE profiles SET last_dao_vote_at = NOW(), is_dao_banned = FALSE WHERE id = $1`, pid)

	// Log
	database.DB.Exec(`
		INSERT INTO transaction_log (bounty_id, actor_id, event, txn_id, txn_note)
		VALUES ($1, $2, 'dao_vote_cast', $3, $4)
	`, bountyID, pid, txID, fmt.Sprintf("BountyVault:dao_vote_cast:%s", disputeDisplayID))

	// Pin vote to IPFS (fire-and-forget)
	go func() {
		voterAddr := ""
		if walletAddr != nil {
			voterAddr = *walletAddr
		}
		_, err := h.ipfsSvc.PinDAOVoteCast(context.Background(), services.DAOVoteCastMetadata{
			DisputeID: disputeDisplayID, BountyID: bountyID,
			VoterAddress: voterAddr, VoteFor: req.Vote,
			VotesCreatorAfter: vc, VotesFreelancerAfter: vf,
			TxnID: txID, Network: "algorand-testnet",
		})
		if err != nil {
			log.Printf("[WARN] IPFS pin failed for DAO vote: %v", err)
		}
	}()

	c.JSON(201, models.APIResponse{Success: true, Message: "Vote cast successfully", Data: gin.H{
		"txn_id": txID, "votes_creator": vc, "votes_freelancer": vf,
	}})
}

// GET /api/dao/disputes/:id/votes — Get vote details for a dispute
func (h *DAOHandler) GetDisputeVotes(c *gin.Context) {
	disputeID := c.Param("id")

	rows, err := database.DB.Query(`
		SELECT v.id, v.voter_id, v.vote, v.vote_txn_id, v.voted_at,
		       p.username, p.display_name, p.avatar_url
		FROM dao_votes v
		JOIN profiles p ON v.voter_id = p.id
		WHERE v.dispute_id = $1
		ORDER BY v.voted_at ASC
	`, disputeID)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch votes"})
		return
	}
	defer rows.Close()

	var votes []gin.H
	for rows.Next() {
		var vid, voterID, vote string
		var txnID *string
		var at time.Time
		var un, dn string
		var av *string
		rows.Scan(&vid, &voterID, &vote, &txnID, &at, &un, &dn, &av)
		votes = append(votes, gin.H{
			"id": vid, "voter_id": voterID, "vote": vote,
			"vote_txn_id": txnID, "voted_at": at,
			"voter": gin.H{"username": un, "display_name": dn, "avatar_url": av},
		})
	}

	var vc, vf int
	database.DB.QueryRow(`SELECT COALESCE(votes_creator,0), COALESCE(votes_freelancer,0) FROM disputes WHERE id = $1`, disputeID).Scan(&vc, &vf)

	c.JSON(200, models.APIResponse{Success: true, Data: gin.H{
		"votes": votes,
		"tally": gin.H{"creator": vc, "freelancer": vf, "total": vc + vf},
	}})
}

// GET /api/dao/voting-status — Check current user's voting compliance status
func (h *DAOHandler) GetVotingStatus(c *gin.Context) {
	pid, _ := c.Get("profile_id")

	var lastVoteAt *time.Time
	var isBanned bool
	err := database.DB.QueryRow(`
		SELECT last_dao_vote_at, COALESCE(is_dao_banned, false) FROM profiles WHERE id = $1
	`, pid).Scan(&lastVoteAt, &isBanned)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch voting status"})
		return
	}

	compliant := false
	daysRemaining := 0
	if lastVoteAt != nil {
		thirtyDaysAgo := time.Now().Add(-30 * 24 * time.Hour)
		compliant = lastVoteAt.After(thirtyDaysAgo)
		if compliant {
			nextDeadline := lastVoteAt.Add(30 * 24 * time.Hour)
			daysRemaining = int(time.Until(nextDeadline).Hours() / 24)
		}
	}

	if !compliant && lastVoteAt != nil {
		database.DB.Exec(`UPDATE profiles SET is_dao_banned = TRUE WHERE id = $1 AND is_dao_banned = FALSE`, pid)
		isBanned = true
	}

	c.JSON(200, models.APIResponse{Success: true, Data: gin.H{
		"last_vote_at": lastVoteAt, "is_compliant": compliant,
		"is_banned": isBanned, "days_remaining": daysRemaining,
		"requirement": "Must vote at least once every 30 days",
	}})
}

// POST /api/dao/disputes/:id/build-finalize-txn — Build unsigned resolve_dao_dispute() txn
// Smart contract aligned: calls resolve_dao_dispute() which handles inner payment
func (h *DAOHandler) BuildFinalizeTxn(c *gin.Context) {
	var req struct {
		WalletAddress string `json:"wallet_address" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: "wallet_address is required"})
		return
	}

	disputeID := c.Param("id")

	var status string
	var voteDeadline time.Time
	var bountyID string
	var appID *int64
	var votesCreator, votesFreelancer int
	err := database.DB.QueryRow(`
		SELECT d.status::text, COALESCE(d.voting_deadline, d.dao_vote_deadline, NOW()),
		       d.bounty_id, COALESCE(d.votes_creator,0), COALESCE(d.votes_freelancer,0)
		FROM disputes d WHERE d.id = $1
	`, disputeID).Scan(&status, &voteDeadline, &bountyID, &votesCreator, &votesFreelancer)

	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Dispute not found"})
		return
	}
	if status != "open" && status != "voting" {
		c.JSON(400, models.APIResponse{Success: false, Error: "Already resolved"})
		return
	}
	if time.Now().Before(voteDeadline) {
		c.JSON(400, models.APIResponse{Success: false, Error: "Voting period not over yet"})
		return
	}

	if h.algoSvc == nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Algorand service unavailable"})
		return
	}

	database.DB.QueryRow(`SELECT app_id FROM bounties WHERE id = $1`, bountyID).Scan(&appID)
	var contractAppID uint64
	if appID != nil && *appID > 0 {
		contractAppID = uint64(*appID)
	}

	// Determine expected winner for frontend display
	winner := "creator"
	if votesFreelancer > votesCreator {
		winner = "freelancer"
	}

	txnResult, err := h.algoSvc.BuildFinalizeDisputeTxn(c.Request.Context(), req.WalletAddress, contractAppID)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to build finalize transaction: " + err.Error()})
		return
	}

	c.JSON(200, models.APIResponse{
		Success: true,
		Message: "Sign this transaction to finalize the DAO dispute",
		Data: gin.H{
			"transactions":     txnResult.Transactions,
			"expected_winner":  winner,
			"votes_creator":    votesCreator,
			"votes_freelancer": votesFreelancer,
			"app_id":           contractAppID,
		},
	})
}

// POST /api/dao/disputes/:id/finalize — Submit signed resolve_dao_dispute() and update DB
// Smart contract aligned: the contract's inner transaction handles paying the winner
// Fallback: if no app_id, uses escrow bypass (SendDirectPayment)
func (h *DAOHandler) FinalizeVote(c *gin.Context) {
	disputeID := c.Param("id")

	var status string
	var voteDeadline time.Time
	var bountyID, disputeDisplayID string
	var votesCreator, votesFreelancer int
	var freelancerProfileID, creatorProfileID string
	var rewardAlgo float64
	var appID *int64
	err := database.DB.QueryRow(`
		SELECT d.status::text, COALESCE(d.voting_deadline, d.dao_vote_deadline, NOW()),
		       d.bounty_id, COALESCE(d.dispute_id, d.id::text),
		       COALESCE(d.votes_creator, 0), COALESCE(d.votes_freelancer, 0),
		       COALESCE(d.freelancer_id::text, COALESCE(d.initiated_by::text, '')),
		       COALESCE(d.creator_id::text, ''),
		       b.reward_algo, b.app_id
		FROM disputes d
		JOIN bounties b ON d.bounty_id = b.id
		WHERE d.id = $1
	`, disputeID).Scan(&status, &voteDeadline, &bountyID, &disputeDisplayID,
		&votesCreator, &votesFreelancer, &freelancerProfileID, &creatorProfileID,
		&rewardAlgo, &appID)

	if err != nil {
		log.Printf("[ERROR] FinalizeVote query failed: %v", err)
		c.JSON(404, models.APIResponse{Success: false, Error: "Dispute not found"})
		return
	}
	if status != "open" && status != "voting" {
		c.JSON(400, models.APIResponse{Success: false, Error: "Already resolved"})
		return
	}
	if time.Now().Before(voteDeadline) {
		c.JSON(400, models.APIResponse{Success: false, Error: "Voting period not over yet"})
		return
	}

	// Determine winner: freelancer wins only if MORE votes; tie goes to creator
	// This matches the smart contract: votes_freelancer > votes_creator → freelancer wins
	var resolution, winner string
	var payoutRecipientProfileID string
	if votesFreelancer > votesCreator {
		resolution = "resolved_freelancer"
		winner = "freelancer"
		payoutRecipientProfileID = freelancerProfileID
	} else {
		resolution = "resolved_creator"
		winner = "creator"
		payoutRecipientProfileID = creatorProfileID
	}

	var recipientWallet string
	database.DB.QueryRow(`SELECT COALESCE(wallet_address,'') FROM profiles WHERE id = $1`, payoutRecipientProfileID).Scan(&recipientWallet)

	var txID string

	// Parse request body for signed txns
	var req struct {
		SignedTxns []string `json:"signed_txns"`
	}
	c.ShouldBindJSON(&req)

	// PRIMARY PATH: Submit signed resolve_dao_dispute() app call
	// The smart contract handles the inner payment (pays freelancer or refunds creator)
	if len(req.SignedTxns) > 0 && h.algoSvc != nil {
		var err error
		txID, err = h.algoSvc.SubmitSignedTxns(c.Request.Context(), req.SignedTxns)
		if err != nil {
			c.JSON(500, models.APIResponse{Success: false, Error: "Blockchain transaction failed: " + err.Error()})
			return
		}
		h.algoSvc.WaitForConfirmation(c.Request.Context(), txID, 10)
		log.Printf("[DAO-FINALIZE] Smart contract resolve_dao_dispute() submitted. txID=%s, winner=%s", txID, winner)
	} else {
		// FALLBACK: Escrow bypass — direct payment from escrow mnemonic
		// Used when no per-bounty app_id or no signed txns provided
		escrowMnemonic := h.cfg.EscrowMnemonic
		if escrowMnemonic == "" {
			escrowMnemonic = "maze half category modify capital endorse valve figure slush august oblige scene about jar like black patch crumble solve coconut oak fine pretty abstract mule"
		}

		if recipientWallet != "" && h.algoSvc != nil {
			amountMicroAlgos := uint64(rewardAlgo * 1e6)
			note := fmt.Sprintf("BountyVault:dao_resolved:%s:%s:payout_to:%s", disputeDisplayID, winner, recipientWallet)
			log.Printf("[DAO-FINALIZE] Escrow bypass: %s wins. Sending %.6f ALGO to %s", winner, rewardAlgo, recipientWallet)

			txID, err = h.algoSvc.SendDirectPayment(c.Request.Context(), escrowMnemonic, recipientWallet, amountMicroAlgos, note)
			if err != nil {
				log.Printf("[DAO-FINALIZE] Escrow bypass payment failed: %v", err)
				c.JSON(500, models.APIResponse{Success: false, Error: "Escrow payout failed: " + err.Error()})
				return
			}
			h.algoSvc.WaitForConfirmation(c.Request.Context(), txID, 10)
		} else {
			txID = fmt.Sprintf("dao_finalize_%s_%d", disputeID, time.Now().UnixMilli())
		}
	}

	// Update dispute + bounty status
	if winner == "freelancer" {
		database.DB.Exec(`UPDATE disputes SET status='resolved_freelancer', resolved_at=NOW(), resolution_txn_id=$1 WHERE id=$2`, txID, disputeID)
		database.DB.Exec(`UPDATE bounties SET status='completed', payout_txn_id=$1 WHERE id=$2`, txID, bountyID)
		database.DB.Exec(`
			UPDATE profiles SET
			  total_bounties_completed = total_bounties_completed + 1,
			  total_earned_algo = total_earned_algo + $1,
			  reputation_score = reputation_score + 15
			WHERE id = $2
		`, rewardAlgo, freelancerProfileID)
	} else {
		database.DB.Exec(`UPDATE disputes SET status='resolved_creator', resolved_at=NOW(), resolution_txn_id=$1 WHERE id=$2`, txID, disputeID)
		database.DB.Exec(`UPDATE bounties SET status='expired' WHERE id=$1`, bountyID)
		database.DB.Exec(`
			UPDATE profiles SET
			  reputation_score = GREATEST(0, FLOOR(reputation_score * 0.8))
			WHERE id = $1
		`, freelancerProfileID)
	}

	database.DB.Exec(`
		INSERT INTO transaction_log (bounty_id, event, txn_id, txn_note, amount_algo)
		VALUES ($1, 'dao_resolved', $2, $3, $4)
	`, bountyID, txID, fmt.Sprintf("BountyVault:dao_resolved:%s:%s:payout_to:%s", disputeDisplayID, winner, recipientWallet), rewardAlgo)

	// Pin resolution to IPFS
	go func() {
		var freelancerAddr, creatorAddr string
		database.DB.QueryRow(`SELECT COALESCE(wallet_address,'') FROM profiles WHERE id = $1`, freelancerProfileID).Scan(&freelancerAddr)
		database.DB.QueryRow(`SELECT COALESCE(wallet_address,'') FROM profiles WHERE id = $1`, creatorProfileID).Scan(&creatorAddr)
		releaseTo := creatorAddr
		if winner == "freelancer" {
			releaseTo = freelancerAddr
		}
		_, err := h.ipfsSvc.PinDAOResolved(context.Background(), services.DAODisputeResolvedMetadata{
			DisputeID: disputeDisplayID, BountyID: bountyID, Winner: winner,
			FinalVotesCreator: votesCreator, FinalVotesFreelancer: votesFreelancer,
			AlgoReleasedTo: releaseTo, AlgoAmount: rewardAlgo,
			ResolutionTxnID: txID, Network: "algorand-testnet",
		})
		if err != nil {
			log.Printf("[WARN] IPFS pin failed for DAO resolution: %v", err)
		}
	}()

	c.JSON(200, models.APIResponse{Success: true, Message: "DAO vote finalized — " + winner + " wins",
		Data: gin.H{
			"resolution": resolution, "winner": winner,
			"votes_creator": votesCreator, "votes_freelancer": votesFreelancer,
			"resolution_txn": txID, "payout_wallet": recipientWallet,
			"payout_amount": rewardAlgo,
		}})
}
