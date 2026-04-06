package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"bountyvault/internal/database"
	"bountyvault/internal/models"
	"bountyvault/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// DAOHandler handles DAO voting endpoints (v3.1)
type DAOHandler struct {
	algoSvc *services.AlgorandService
	ipfsSvc *services.IPFSService
}

func NewDAOHandler(algoSvc *services.AlgorandService, ipfsSvc *services.IPFSService) *DAOHandler {
	return &DAOHandler{algoSvc: algoSvc, ipfsSvc: ipfsSvc}
}

// GET /api/dao/disputes — List all active disputes with vote tallies
func (h *DAOHandler) ListActiveDisputes(c *gin.Context) {
	rows, err := database.DB.Query(`
		SELECT d.id, d.dispute_id, d.bounty_id, d.freelancer_description, d.status,
		       d.voting_deadline, d.votes_creator, d.votes_freelancer, d.created_at,
		       b.title, b.reward_algo, b.deadline,
		       pf.username AS freelancer_name,
		       pc.username AS creator_name
		FROM disputes d
		JOIN bounties b ON d.bounty_id = b.id
		JOIN profiles pf ON d.freelancer_id = pf.id
		JOIN profiles pc ON d.creator_id = pc.id
		WHERE d.status = 'open'
		ORDER BY d.created_at DESC
	`)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch disputes"})
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

// POST /api/dao/vote — Cast a DAO vote on a dispute
func (h *DAOHandler) CastVote(c *gin.Context) {
	pid, _ := c.Get("profile_id")
	var req models.CastDAOVoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: "Invalid request: " + err.Error()})
		return
	}

	disputeID := c.Param("id")

	// Verify dispute exists and voting is open
	var status string
	var voteDeadline time.Time
	var bountyCreatorID, freelancerID, bountyID string
	var disputeDisplayID string
	err := database.DB.QueryRow(`
		SELECT d.status, d.voting_deadline, d.creator_id, d.freelancer_id, d.bounty_id, d.dispute_id
		FROM disputes d WHERE d.id = $1
	`, disputeID).Scan(&status, &voteDeadline, &bountyCreatorID, &freelancerID, &bountyID, &disputeDisplayID)

	if err == sql.ErrNoRows {
		c.JSON(404, models.APIResponse{Success: false, Error: "Dispute not found"})
		return
	}
	if status != "open" {
		c.JSON(400, models.APIResponse{Success: false, Error: "Dispute voting is closed"})
		return
	}
	if time.Now().After(voteDeadline) {
		c.JSON(400, models.APIResponse{Success: false, Error: "Voting period has expired"})
		return
	}

	// Creator and disputing freelancer cannot vote
	if bountyCreatorID == pid.(string) {
		c.JSON(403, models.APIResponse{Success: false, Error: "Bounty creator cannot vote"})
		return
	}
	if freelancerID == pid.(string) {
		c.JSON(403, models.APIResponse{Success: false, Error: "Disputing freelancer cannot vote"})
		return
	}

	// Submit DAO vote transaction to Algorand
	txID, err := h.algoSvc.SubmitSignedTxns(c.Request.Context(), req.SignedTxns)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Blockchain transaction failed: " + err.Error()})
		return
	}
	h.algoSvc.WaitForConfirmation(c.Request.Context(), txID, 10)

	// Insert vote (UNIQUE constraint prevents double voting)
	var walletAddr *string
	database.DB.QueryRow(`SELECT wallet_address FROM profiles WHERE id=$1`, pid).Scan(&walletAddr)

	vid := uuid.New()
	_, err = database.DB.Exec(`
		INSERT INTO dao_votes (id, dispute_id, voter_id, vote, vote_txn_id)
		VALUES ($1, $2, $3, $4, $5)
	`, vid, disputeID, pid, req.Vote, txID)
	if err != nil {
		c.JSON(409, models.APIResponse{Success: false, Error: "You have already voted on this dispute"})
		return
	}

	// Update tally on dispute record
	if req.Vote == "creator" {
		database.DB.Exec(`UPDATE disputes SET votes_creator = votes_creator + 1 WHERE id = $1`, disputeID)
	} else {
		database.DB.Exec(`UPDATE disputes SET votes_freelancer = votes_freelancer + 1 WHERE id = $1`, disputeID)
	}

	// Get updated tallies
	var vc, vf int
	database.DB.QueryRow(`SELECT votes_creator, votes_freelancer FROM disputes WHERE id = $1`, disputeID).Scan(&vc, &vf)

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
			DisputeID:              disputeDisplayID,
			BountyID:               bountyID,
			VoterAddress:           voterAddr,
			VoteFor:                req.Vote,
			VotesCreatorAfter:      vc,
			VotesFreelancerAfter:   vf,
			TxnID:                  txID,
			Network:                "algorand-testnet",
		})
		if err != nil {
			log.Printf("[WARN] IPFS pin failed for DAO vote: %v", err)
		}
	}()

	c.JSON(201, models.APIResponse{Success: true, Message: "Vote cast successfully", Data: gin.H{
		"txn_id":           txID,
		"votes_creator":    vc,
		"votes_freelancer": vf,
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

	// Get tally from dispute
	var vc, vf int
	database.DB.QueryRow(`SELECT votes_creator, votes_freelancer FROM disputes WHERE id = $1`, disputeID).Scan(&vc, &vf)

	c.JSON(200, models.APIResponse{Success: true, Data: gin.H{
		"votes": votes,
		"tally": gin.H{"creator": vc, "freelancer": vf, "total": vc + vf},
	}})
}

// POST /api/dao/disputes/:id/finalize — Finalize DAO vote (permissionless after deadline)
func (h *DAOHandler) FinalizeVote(c *gin.Context) {
	disputeID := c.Param("id")

	var status string
	var voteDeadline time.Time
	var bountyID, disputeDisplayID string
	var votesCreator, votesFreelancer int
	var freelancerProfileID, creatorProfileID string
	var rewardAlgo float64
	err := database.DB.QueryRow(`
		SELECT d.status, d.voting_deadline, d.bounty_id, d.dispute_id,
		       d.votes_creator, d.votes_freelancer, d.freelancer_id, d.creator_id,
		       b.reward_algo
		FROM disputes d
		JOIN bounties b ON d.bounty_id = b.id
		WHERE d.id = $1
	`, disputeID).Scan(&status, &voteDeadline, &bountyID, &disputeDisplayID,
		&votesCreator, &votesFreelancer, &freelancerProfileID, &creatorProfileID, &rewardAlgo)

	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Dispute not found"})
		return
	}
	if status != "open" {
		c.JSON(400, models.APIResponse{Success: false, Error: "Already resolved"})
		return
	}
	if time.Now().Before(voteDeadline) {
		c.JSON(400, models.APIResponse{Success: false, Error: "Voting period not over yet"})
		return
	}

	// Submit resolve_dao_dispute transaction
	var req struct {
		SignedTxns []string `json:"signed_txns" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	txID, err := h.algoSvc.SubmitSignedTxns(c.Request.Context(), req.SignedTxns)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Blockchain transaction failed: " + err.Error()})
		return
	}
	h.algoSvc.WaitForConfirmation(c.Request.Context(), txID, 10)

	// Resolution: freelancer wins if more votes, tie goes to creator
	var resolution, winner string
	if votesFreelancer > votesCreator {
		resolution = "resolved_freelancer"
		winner = "freelancer"
		database.DB.Exec(`UPDATE disputes SET status='resolved_freelancer', resolved_at=NOW(), resolution_txn_id=$1 WHERE id=$2`, txID, disputeID)
		database.DB.Exec(`UPDATE bounties SET status='completed', payout_txn_id=$1 WHERE id=$2`, txID, bountyID)
		// Credit freelancer
		database.DB.Exec(`
			UPDATE profiles SET
			  total_bounties_completed = total_bounties_completed + 1,
			  total_earned_algo = total_earned_algo + $1,
			  reputation_score = reputation_score + 15
			WHERE id = $2
		`, rewardAlgo, freelancerProfileID)
	} else {
		resolution = "resolved_creator"
		winner = "creator"
		database.DB.Exec(`UPDATE disputes SET status='resolved_creator', resolved_at=NOW(), resolution_txn_id=$1 WHERE id=$2`, txID, disputeID)
		database.DB.Exec(`UPDATE bounties SET status='expired' WHERE id=$1`, bountyID)
	}

	database.DB.Exec(`
		INSERT INTO transaction_log (bounty_id, event, txn_id, txn_note, amount_algo)
		VALUES ($1, 'dao_resolved', $2, $3, $4)
	`, bountyID, txID, fmt.Sprintf("BountyVault:dao_resolved:%s:%s", disputeDisplayID, winner), rewardAlgo)

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
			DisputeID:            disputeDisplayID,
			BountyID:             bountyID,
			Winner:               winner,
			FinalVotesCreator:    votesCreator,
			FinalVotesFreelancer: votesFreelancer,
			AlgoReleasedTo:       releaseTo,
			AlgoAmount:           rewardAlgo,
			ResolutionTxnID:      txID,
			Network:              "algorand-testnet",
		})
		if err != nil {
			log.Printf("[WARN] IPFS pin failed for DAO resolution: %v", err)
		}
	}()

	c.JSON(200, models.APIResponse{Success: true, Message: "DAO vote finalized",
		Data: gin.H{
			"resolution":       resolution,
			"winner":           winner,
			"votes_creator":    votesCreator,
			"votes_freelancer": votesFreelancer,
			"resolution_txn":   txID,
		}})
}
