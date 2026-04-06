package handlers

import (
	"time"

	"bountyvault/internal/database"
	"bountyvault/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// DashboardHandler handles user-specific dashboard data
type DashboardHandler struct{}

func NewDashboardHandler() *DashboardHandler {
	return &DashboardHandler{}
}

// GET /api/dashboard/stats — User-specific statistics
func (h *DashboardHandler) GetUserStats(c *gin.Context) {
	pid, _ := c.Get("profile_id")
	role, _ := c.Get("role")

	var rep, bCreated, bCompleted, streak, totalRatings int
	var earnings, avgRating float64
	err := database.DB.QueryRow(`
		SELECT reputation_score, total_bounties_created, total_bounties_completed,
		       total_earned_algo, streak_count, avg_rating, total_ratings
		FROM profiles WHERE id = $1
	`, pid).Scan(&rep, &bCreated, &bCompleted, &earnings, &streak, &avgRating, &totalRatings)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch stats"})
		return
	}

	// Active bounties (open or in_progress) created by this user
	var activeBounties int
	database.DB.QueryRow(`
		SELECT COUNT(*) FROM bounties 
		WHERE creator_id = $1 AND status IN ('open', 'in_progress')
	`, pid).Scan(&activeBounties)

	// Active submissions (pending) by this user
	var activeSubmissions int
	database.DB.QueryRow(`
		SELECT COUNT(*) FROM submissions
		WHERE freelancer_id = $1 AND status = 'pending'
	`, pid).Scan(&activeSubmissions)

	// Total submissions by this user
	var totalSubmissions int
	database.DB.QueryRow(`SELECT COUNT(*) FROM submissions WHERE freelancer_id = $1`, pid).Scan(&totalSubmissions)

	// Approved submissions (wins)
	var approvedSubmissions int
	database.DB.QueryRow(`SELECT COUNT(*) FROM submissions WHERE freelancer_id = $1 AND status = 'approved'`, pid).Scan(&approvedSubmissions)

	// Win rate
	winRate := 0.0
	if totalSubmissions > 0 {
		winRate = float64(approvedSubmissions) / float64(totalSubmissions) * 100
	}

	// Unread notifications count (table may not exist in all schemas)
	var unreadNotifs int
	_ = database.DB.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = FALSE`, pid).Scan(&unreadNotifs)

	// Total spent (for creators)
	var totalSpent float64
	database.DB.QueryRow(`
		SELECT COALESCE(SUM(reward_algo), 0) FROM bounties 
		WHERE creator_id = $1 AND status = 'completed'
	`, pid).Scan(&totalSpent)

	// Open disputes count
	var openDisputes int
	database.DB.QueryRow(`
		SELECT COUNT(*) FROM disputes 
		WHERE initiated_by = $1 AND status = 'open'
	`, pid).Scan(&openDisputes)

	// Working bounties count (for freelancers — bounties where they have a pending submission)
	var workingBounties int
	database.DB.QueryRow(`
		SELECT COUNT(*) FROM submissions s
		JOIN bounties b ON s.bounty_id = b.id
		WHERE s.freelancer_id = $1 AND s.status = 'pending' AND b.status IN ('open', 'in_progress')
	`, pid).Scan(&workingBounties)

	// Pending acceptances count
	var pendingAcceptances int
	if role.(string) == "creator" {
		// Creator: count pending acceptance requests on their bounties
		database.DB.QueryRow(`
			SELECT COUNT(*) FROM bounty_acceptances a
			JOIN bounties b ON a.bounty_id = b.id
			WHERE b.creator_id = $1 AND a.status = 'pending'
		`, pid).Scan(&pendingAcceptances)
	} else {
		// Freelancer: count their own pending acceptances
		database.DB.QueryRow(`
			SELECT COUNT(*) FROM bounty_acceptances
			WHERE freelancer_id = $1 AND status = 'pending'
		`, pid).Scan(&pendingAcceptances)
	}

	c.JSON(200, models.APIResponse{Success: true, Data: gin.H{
		"reputation_score":        rep,
		"total_bounties_created":  bCreated,
		"total_bounties_completed": bCompleted,
		"total_earnings_algo":     earnings,
		"streak_count":            streak,
		"avg_rating":              avgRating,
		"total_ratings":           totalRatings,
		"active_bounties":         activeBounties,
		"active_submissions":      activeSubmissions,
		"total_submissions":       totalSubmissions,
		"approved_submissions":    approvedSubmissions,
		"win_rate":                winRate,
		"unread_notifications":    unreadNotifs,
		"total_spent_algo":        totalSpent,
		"open_disputes":           openDisputes,
		"working_bounties":        workingBounties,
		"pending_acceptances":     pendingAcceptances,
		"role":                    role,
	}})
}


// GET /api/dashboard/my-bounties — Bounties created by the authenticated user
func (h *DashboardHandler) GetMyBounties(c *gin.Context) {
	pid, _ := c.Get("profile_id")

	rows, err := database.DB.Query(`
		SELECT b.id, b.title, b.description, b.reward_algo, b.deadline, b.status,
		       b.max_submissions, b.tags, b.created_at,
		       (SELECT COUNT(*) FROM submissions s WHERE s.bounty_id = b.id) as sub_count
		FROM bounties b
		WHERE b.creator_id = $1
		ORDER BY b.created_at DESC
		LIMIT 20
	`, pid)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch bounties"})
		return
	}
	defer rows.Close()

	var bounties []gin.H
	for rows.Next() {
		var id, title, desc, status string
		var reward float64
		var deadline, createdAt time.Time
		var maxSubs, subCount int
		var tags []string

		rows.Scan(&id, &title, &desc, &reward, &deadline, &status,
			&maxSubs, pq.Array(&tags), &createdAt, &subCount)

		bounties = append(bounties, gin.H{
			"id":               id,
			"title":            title,
			"description":      desc,
			"reward_algo":      reward,
			"deadline":         deadline,
			"status":           status,
			"max_submissions":  maxSubs,
			"tags":             tags,
			"created_at":       createdAt,
			"submission_count": subCount,
		})
	}

	if bounties == nil {
		bounties = []gin.H{}
	}

	c.JSON(200, models.APIResponse{Success: true, Data: bounties})
}

// GET /api/dashboard/my-submissions — Submissions made by the authenticated user
func (h *DashboardHandler) GetMySubmissions(c *gin.Context) {
	pid, _ := c.Get("profile_id")

	rows, err := database.DB.Query(`
		SELECT s.id, s.bounty_id, s.status, s.rejection_feedback, s.created_at,
		       s.description,
		       b.title as bounty_title, b.reward_algo, b.status as bounty_status,
		       b.deadline
		FROM submissions s
		JOIN bounties b ON s.bounty_id = b.id
		WHERE s.freelancer_id = $1
		ORDER BY s.created_at DESC
		LIMIT 20
	`, pid)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch submissions"})
		return
	}
	defer rows.Close()

	var submissions []gin.H
	for rows.Next() {
		var id, bountyID, status string
		var bountyTitle, bountyStatus string
		var reward float64
		var feedback, description *string
		var submittedAt, deadline time.Time

		rows.Scan(&id, &bountyID, &status, &feedback, &submittedAt,
			&description, &bountyTitle, &reward, &bountyStatus, &deadline)

		submissions = append(submissions, gin.H{
			"id":              id,
			"bounty_id":       bountyID,
			"status":          status,
			"feedback":        feedback,
			"submitted_at":    submittedAt,
			"description":     description,
			"bounty_title":    bountyTitle,
			"bounty_reward":   reward,
			"bounty_status":   bountyStatus,
			"bounty_deadline": deadline,
		})
	}

	if submissions == nil {
		submissions = []gin.H{}
	}

	c.JSON(200, models.APIResponse{Success: true, Data: submissions})
}

// GET /api/dashboard/working-bounties — Bounties where freelancer is accepted and can submit work
func (h *DashboardHandler) GetWorkingBounties(c *gin.Context) {
	pid, _ := c.Get("profile_id")

	rows, err := database.DB.Query(`
		SELECT b.id, b.title, b.description, b.reward_algo, b.deadline, b.status,
		       b.max_submissions, b.submissions_remaining, b.tags, b.created_at,
		       COALESCE(
		         (SELECT s.status FROM submissions s
		          WHERE s.bounty_id = b.id AND s.freelancer_id = $1
		          ORDER BY s.created_at DESC LIMIT 1), 'none'
		       ) as latest_sub_status,
		       COALESCE(
		         (SELECT s.id FROM submissions s
		          WHERE s.bounty_id = b.id AND s.freelancer_id = $1
		          ORDER BY s.created_at DESC LIMIT 1), ''
		       ) as latest_sub_id,
		       COALESCE(
		         (SELECT s.created_at FROM submissions s
		          WHERE s.bounty_id = b.id AND s.freelancer_id = $1
		          ORDER BY s.created_at DESC LIMIT 1), b.created_at
		       ) as latest_sub_at,
		       COALESCE(
		         (SELECT s.rejection_feedback FROM submissions s
		          WHERE s.bounty_id = b.id AND s.freelancer_id = $1 AND s.status = 'rejected'
		          ORDER BY s.created_at DESC LIMIT 1), ''
		       ) as last_rejection_feedback,
		       (SELECT COUNT(*) FROM submissions sub WHERE sub.bounty_id = b.id) as sub_count,
		       p.username as creator_username
		FROM bounties b
		JOIN profiles p ON b.creator_id = p.id
		WHERE b.accepted_freelancer_id = $1
		  AND b.status IN ('in_progress', 'expired')
		ORDER BY b.updated_at DESC
		LIMIT 20
	`, pid)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch working bounties"})
		return
	}
	defer rows.Close()

	var bounties []gin.H
	for rows.Next() {
		var id, title, desc, bStatus, latestSubStatus, latestSubID, creatorUsername string
		var lastRejFeedback string
		var reward float64
		var deadline, createdAt, latestSubAt time.Time
		var maxSubs, subsRemaining, subCount int
		var tags []string

		rows.Scan(&id, &title, &desc, &reward, &deadline, &bStatus,
			&maxSubs, &subsRemaining, pq.Array(&tags), &createdAt,
			&latestSubStatus, &latestSubID, &latestSubAt,
			&lastRejFeedback, &subCount, &creatorUsername)

		// Has submitted at least once?
		hasSubmitted := latestSubStatus != "none"
		// Can submit if: bounty in_progress, no pending submission, and remaining slots > 0
		canSubmit := bStatus == "in_progress" &&
			(latestSubStatus == "none" || latestSubStatus == "rejected") &&
			subsRemaining > 0
		// Can resubmit if latest was rejected and slots remain
		canResubmit := latestSubStatus == "rejected" && subsRemaining > 0
		// Can let go or dispute if expired AND at least 1 submission was made
		canLetGo := bStatus == "expired" && hasSubmitted
		canDispute := bStatus == "expired" && hasSubmitted

		bounties = append(bounties, gin.H{
			"id":                    id,
			"title":                 title,
			"description":           desc,
			"reward_algo":           reward,
			"deadline":              deadline,
			"status":                bStatus,
			"max_submissions":       maxSubs,
			"submissions_remaining": subsRemaining,
			"tags":                  tags,
			"created_at":            createdAt,
			"submission_status":     latestSubStatus,
			"submitted_at":          latestSubAt,
			"submission_id":         latestSubID,
			"submission_count":      subCount,
			"creator_username":      creatorUsername,
			"rejection_feedback":    lastRejFeedback,
			"can_submit":            canSubmit,
			"can_resubmit":          canResubmit,
			"can_let_go":            canLetGo,
			"can_dispute":           canDispute,
			"has_submitted":         hasSubmitted,
		})
	}

	if bounties == nil {
		bounties = []gin.H{}
	}

	c.JSON(200, models.APIResponse{Success: true, Data: bounties})
}

// GET /api/dashboard/disputes — Disputes involving the authenticated user
func (h *DashboardHandler) GetMyDisputes(c *gin.Context) {
	pid, _ := c.Get("profile_id")

	rows, err := database.DB.Query(`
		SELECT d.id, d.bounty_id, d.reason, d.status, d.created_at,
		       d.evidence_ipfs_cid, d.dao_vote_deadline, d.auto_refund_after,
		       b.title as bounty_title, b.reward_algo,
		       p.username as initiated_by_username,
		       (SELECT COUNT(*) FILTER(WHERE vote='approve') FROM dao_votes WHERE dispute_id = d.id) as approve_votes,
		       (SELECT COUNT(*) FILTER(WHERE vote='reject') FROM dao_votes WHERE dispute_id = d.id) as reject_votes,
		       (SELECT COUNT(*) FROM dao_votes WHERE dispute_id = d.id) as total_votes
		FROM disputes d
		JOIN bounties b ON d.bounty_id = b.id
		JOIN profiles p ON d.initiated_by = p.id
		WHERE d.initiated_by = $1
		   OR b.creator_id = $1
		   OR EXISTS (SELECT 1 FROM submissions s WHERE s.bounty_id = d.bounty_id AND s.freelancer_id = $1)
		ORDER BY d.created_at DESC
		LIMIT 20
	`, pid)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch disputes"})
		return
	}
	defer rows.Close()

	var disputes []gin.H
	for rows.Next() {
		var id, bountyID, reason, status, bountyTitle, initiatedByUsername string
		var createdAt time.Time
		var evidenceCid *string
		var voteDeadline, refundAfter *time.Time
		var reward float64
		var approveVotes, rejectVotes, totalVotes int

		rows.Scan(&id, &bountyID, &reason, &status, &createdAt,
			&evidenceCid, &voteDeadline, &refundAfter,
			&bountyTitle, &reward,
			&initiatedByUsername,
			&approveVotes, &rejectVotes, &totalVotes)

		disputes = append(disputes, gin.H{
			"id":                    id,
			"bounty_id":             bountyID,
			"reason":                reason,
			"status":                status,
			"created_at":            createdAt,
			"evidence_ipfs_cid":     evidenceCid,
			"dao_vote_deadline":     voteDeadline,
			"auto_refund_after":     refundAfter,
			"bounty_title":          bountyTitle,
			"bounty_reward":         reward,
			"initiated_by_username": initiatedByUsername,
			"votes": gin.H{
				"approve": approveVotes,
				"reject":  rejectVotes,
				"total":   totalVotes,
			},
		})
	}

	if disputes == nil {
		disputes = []gin.H{}
	}

	c.JSON(200, models.APIResponse{Success: true, Data: disputes})
}

// GET /api/bounties/:id/status-history — Status update history for a bounty
func (h *DashboardHandler) GetBountyStatusHistory(c *gin.Context) {
	bountyID := c.Param("id")
	if _, err := uuid.Parse(bountyID); err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: "Invalid bounty ID"})
		return
	}

	rows, err := database.DB.Query(`
		SELECT su.id, su.bounty_id, su.old_status, su.new_status, su.note, su.created_at,
		       p.username as updated_by_username, p.role as updated_by_role
		FROM bounty_status_updates su
		JOIN profiles p ON su.updated_by = p.id
		WHERE su.bounty_id = $1
		ORDER BY su.created_at DESC
		LIMIT 50
	`, bountyID)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch status history"})
		return
	}
	defer rows.Close()

	var updates []gin.H
	for rows.Next() {
		var id, bID, newStatus, username, role string
		var oldStatus, note *string
		var createdAt time.Time

		rows.Scan(&id, &bID, &oldStatus, &newStatus, &note, &createdAt, &username, &role)

		updates = append(updates, gin.H{
			"id":                  id,
			"bounty_id":           bID,
			"old_status":          oldStatus,
			"new_status":          newStatus,
			"note":                note,
			"created_at":          createdAt,
			"updated_by_username": username,
			"updated_by_role":     role,
		})
	}

	if updates == nil {
		updates = []gin.H{}
	}

	c.JSON(200, models.APIResponse{Success: true, Data: updates})
}

// POST /api/bounties/:id/status-update — Freelancer/creator posts a progress update
func (h *DashboardHandler) UpdateBountyStatus(c *gin.Context) {
	bountyID := c.Param("id")
	pid, _ := c.Get("profile_id")

	bID, err := uuid.Parse(bountyID)
	if err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: "Invalid bounty ID"})
		return
	}

	var req models.UpdateBountyStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, models.APIResponse{Success: false, Error: "Invalid request: " + err.Error()})
		return
	}

	// Verify the user is associated with this bounty (creator or freelancer with submission)
	var currentStatus string
	err = database.DB.QueryRow(`SELECT status FROM bounties WHERE id = $1`, bID).Scan(&currentStatus)
	if err != nil {
		c.JSON(404, models.APIResponse{Success: false, Error: "Bounty not found"})
		return
	}

	// Check authorization: user must be creator or have a submission on this bounty
	var authorized bool
	err = database.DB.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM bounties WHERE id = $1 AND creator_id = $2
			UNION ALL
			SELECT 1 FROM submissions WHERE bounty_id = $1 AND freelancer_id = $2
		)
	`, bID, pid).Scan(&authorized)
	if err != nil || !authorized {
		c.JSON(403, models.APIResponse{Success: false, Error: "Not authorized to update this bounty status"})
		return
	}

	// Insert status update
	var note *string
	if req.Note != "" {
		note = &req.Note
	}
	_, err = database.DB.Exec(`
		INSERT INTO bounty_status_updates (bounty_id, updated_by, old_status, new_status, note)
		VALUES ($1, $2, $3, $4, $5)
	`, bID, pid, currentStatus, req.Status, note)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to record status update"})
		return
	}

	c.JSON(200, models.APIResponse{Success: true, Message: "Status update recorded"})
}

// GET /api/dashboard/pending-acceptances — Creator sees all pending acceptance requests across their bounties
func (h *DashboardHandler) GetPendingAcceptances(c *gin.Context) {
	pid, _ := c.Get("profile_id")

	rows, err := database.DB.Query(`
		SELECT a.id, a.bounty_id, a.freelancer_id, a.status, a.message, a.creator_note, a.created_at,
		       b.title as bounty_title, b.reward_algo, b.deadline, b.status as bounty_status, b.tags,
		       p.id as p_id, p.username, p.display_name, p.avatar_url, p.reputation_score, p.bio,
		       p.total_bounties_completed, p.avg_rating, p.total_ratings
		FROM bounty_acceptances a
		JOIN bounties b ON a.bounty_id = b.id
		JOIN profiles p ON a.freelancer_id = p.id
		WHERE b.creator_id = $1
		ORDER BY
			CASE WHEN a.status = 'pending' THEN 0 ELSE 1 END,
			a.created_at DESC
		LIMIT 50
	`, pid)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch pending acceptances"})
		return
	}
	defer rows.Close()

	var acceptances []gin.H
	for rows.Next() {
		var aID, aBountyID, aFreelancerID, aStatus string
		var aMsg, aNote *string
		var aCreated time.Time
		var bTitle, bStatus string
		var bReward float64
		var bDeadline time.Time
		var bTags []string
		var pID, pUsername string
		var pDisplayName, pAvatarURL, pBio *string
		var pRep, pCompleted, pTotalRatings int
		var pAvgRating float64

		rows.Scan(
			&aID, &aBountyID, &aFreelancerID, &aStatus, &aMsg, &aNote, &aCreated,
			&bTitle, &bReward, &bDeadline, &bStatus, pq.Array(&bTags),
			&pID, &pUsername, &pDisplayName, &pAvatarURL, &pRep, &pBio,
			&pCompleted, &pAvgRating, &pTotalRatings,
		)

		acceptances = append(acceptances, gin.H{
			"id":            aID,
			"bounty_id":     aBountyID,
			"freelancer_id": aFreelancerID,
			"status":        aStatus,
			"message":       aMsg,
			"creator_note":  aNote,
			"created_at":    aCreated,
			"bounty": gin.H{
				"title":      bTitle,
				"reward_algo": bReward,
				"deadline":   bDeadline,
				"status":     bStatus,
				"tags":       bTags,
			},
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

	if acceptances == nil {
		acceptances = []gin.H{}
	}

	c.JSON(200, models.APIResponse{Success: true, Data: acceptances})
}

// GET /api/dashboard/my-acceptances — Freelancer sees their own acceptance requests
func (h *DashboardHandler) GetMyAcceptances(c *gin.Context) {
	pid, _ := c.Get("profile_id")

	rows, err := database.DB.Query(`
		SELECT a.id, a.bounty_id, a.status, a.message, a.creator_note, a.created_at,
		       b.title as bounty_title, b.reward_algo, b.deadline, b.status as bounty_status, b.tags,
		       p.username as creator_username
		FROM bounty_acceptances a
		JOIN bounties b ON a.bounty_id = b.id
		JOIN profiles p ON b.creator_id = p.id
		WHERE a.freelancer_id = $1
		ORDER BY a.created_at DESC
		LIMIT 30
	`, pid)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch acceptances"})
		return
	}
	defer rows.Close()

	var acceptances []gin.H
	for rows.Next() {
		var aID, aBountyID, aStatus string
		var aMsg, aNote *string
		var aCreated time.Time
		var bTitle, bStatus, creatorUsername string
		var bReward float64
		var bDeadline time.Time
		var bTags []string

		rows.Scan(
			&aID, &aBountyID, &aStatus, &aMsg, &aNote, &aCreated,
			&bTitle, &bReward, &bDeadline, &bStatus, pq.Array(&bTags),
			&creatorUsername,
		)

		acceptances = append(acceptances, gin.H{
			"id":           aID,
			"bounty_id":    aBountyID,
			"status":       aStatus,
			"message":      aMsg,
			"creator_note": aNote,
			"created_at":   aCreated,
			"bounty_title":    bTitle,
			"bounty_reward":   bReward,
			"bounty_deadline": bDeadline,
			"bounty_status":   bStatus,
			"bounty_tags":     bTags,
			"creator_username": creatorUsername,
		})
	}

	if acceptances == nil {
		acceptances = []gin.H{}
	}

	c.JSON(200, models.APIResponse{Success: true, Data: acceptances})
}


// GET /api/dashboard/transactions — Transaction log for the authenticated user
func (h *DashboardHandler) GetTransactionLog(c *gin.Context) {
	pid, _ := c.Get("profile_id")

	rows, err := database.DB.Query(`
		SELECT t.id, t.bounty_id, t.actor_id, t.event, t.txn_id, t.txn_note,
		       t.ipfs_metadata_cid, t.amount_algo, t.created_at,
		       COALESCE(p.username, '') as actor_username,
		       COALESCE(b.title, '') as bounty_title,
		       COALESCE(b.bounty_id, '') as bounty_display_id
		FROM transaction_log t
		LEFT JOIN profiles p ON t.actor_id = p.id
		LEFT JOIN bounties b ON t.bounty_id = b.id
		WHERE t.actor_id = $1
		   OR t.bounty_id IN (SELECT id FROM bounties WHERE creator_id = $1)
		   OR t.bounty_id IN (SELECT id FROM bounties WHERE accepted_freelancer_id = $1)
		ORDER BY t.created_at DESC
		LIMIT 50
	`, pid)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch transactions"})
		return
	}
	defer rows.Close()

	var txns []gin.H
	for rows.Next() {
		var id, event string
		var bountyID, actorID, txnID, txnNote, ipfsCID *string
		var amountAlgo *float64
		var createdAt time.Time
		var actorUsername, bountyTitle, bountyDisplayID string

		rows.Scan(&id, &bountyID, &actorID, &event, &txnID, &txnNote,
			&ipfsCID, &amountAlgo, &createdAt,
			&actorUsername, &bountyTitle, &bountyDisplayID)

		// Build IPFS gateway URL if CID present
		var ipfsURL *string
		if ipfsCID != nil && *ipfsCID != "" {
			url := "https://gateway.pinata.cloud/ipfs/" + *ipfsCID
			ipfsURL = &url
		}

		txns = append(txns, gin.H{
			"id":               id,
			"bounty_id":        bountyID,
			"actor_id":         actorID,
			"event":            event,
			"txn_id":           txnID,
			"txn_note":         txnNote,
			"ipfs_metadata_cid": ipfsCID,
			"ipfs_gateway_url": ipfsURL,
			"amount_algo":      amountAlgo,
			"created_at":       createdAt,
			"actor_username":   actorUsername,
			"bounty_title":     bountyTitle,
			"bounty_display_id": bountyDisplayID,
		})
	}

	if txns == nil {
		txns = []gin.H{}
	}

	c.JSON(200, models.APIResponse{Success: true, Data: txns})
}
