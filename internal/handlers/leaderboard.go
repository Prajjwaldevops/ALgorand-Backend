package handlers

import (
	"bountyvault/internal/database"
	"bountyvault/internal/models"

	"github.com/gin-gonic/gin"
)

// LeaderboardHandler handles reputation and ranking endpoints
type LeaderboardHandler struct{}

func NewLeaderboardHandler() *LeaderboardHandler {
	return &LeaderboardHandler{}
}

// GET /api/leaderboard — Top workers by reputation
func (h *LeaderboardHandler) GetLeaderboard(c *gin.Context) {
	limit := c.DefaultQuery("limit", "20")

	rows, err := database.DB.Query(`
		SELECT p.id, p.username, p.display_name, p.avatar_url, p.wallet_address,
		       p.reputation_score, p.total_bounties_completed, p.total_bounties_created,
		       p.total_earned_algo, p.avg_rating, p.streak_count,
		       RANK() OVER (ORDER BY p.reputation_score DESC) as rank
		FROM profiles p
		WHERE p.total_bounties_completed > 0 OR p.total_bounties_created > 0
		ORDER BY p.reputation_score DESC
		LIMIT $1
	`, limit)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed to fetch leaderboard"})
		return
	}
	defer rows.Close()

	var entries []gin.H
	for rows.Next() {
		var id, username string
		var displayName, avatarURL, walletAddr *string
		var repScore, bCompleted, bCreated, streak int
		var earnings, avgRating float64
		var rank int

		rows.Scan(&id, &username, &displayName, &avatarURL, &walletAddr,
			&repScore, &bCompleted, &bCreated, &earnings, &avgRating, &streak, &rank)

		entries = append(entries, gin.H{
			"rank": rank, "id": id, "username": username,
			"display_name": displayName, "avatar_url": avatarURL,
			"wallet_address": walletAddr, "reputation_score": repScore,
			"bounties_completed": bCompleted, "bounties_created": bCreated,
			"total_earnings": earnings, "avg_rating": avgRating,
			"streak": streak,
		})
	}

	c.JSON(200, models.APIResponse{Success: true, Data: entries})
}

// GET /api/leaderboard/top-creators — Top bounty creators
func (h *LeaderboardHandler) GetTopCreators(c *gin.Context) {
	rows, err := database.DB.Query(`
		SELECT p.id, p.username, p.display_name, p.avatar_url,
		       p.total_bounties_created,
		       COALESCE(SUM(b.reward_algo), 0) as total_posted
		FROM profiles p
		LEFT JOIN bounties b ON p.id = b.creator_id
		WHERE p.total_bounties_created > 0
		GROUP BY p.id
		ORDER BY total_posted DESC
		LIMIT 20
	`)
	if err != nil {
		c.JSON(500, models.APIResponse{Success: false, Error: "Failed"})
		return
	}
	defer rows.Close()

	var entries []gin.H
	for rows.Next() {
		var id, username string
		var dn, av *string
		var created int
		var posted float64
		rows.Scan(&id, &username, &dn, &av, &created, &posted)
		entries = append(entries, gin.H{
			"id": id, "username": username, "display_name": dn,
			"avatar_url": av, "bounties_created": created, "total_posted_algo": posted,
		})
	}
	c.JSON(200, models.APIResponse{Success: true, Data: entries})
}

// GET /api/stats — Platform statistics
func (h *LeaderboardHandler) GetPlatformStats(c *gin.Context) {
	var totalBounties, openBounties, completedBounties, totalUsers int
	var totalRewardsLocked, totalRewardsPaid float64

	database.DB.QueryRow(`SELECT COUNT(*) FROM bounties`).Scan(&totalBounties)
	database.DB.QueryRow(`SELECT COUNT(*) FROM bounties WHERE status='open'`).Scan(&openBounties)
	database.DB.QueryRow(`SELECT COUNT(*) FROM bounties WHERE status='completed'`).Scan(&completedBounties)
	database.DB.QueryRow(`SELECT COUNT(*) FROM profiles`).Scan(&totalUsers)
	database.DB.QueryRow(`SELECT COALESCE(SUM(reward_algo),0) FROM bounties WHERE status IN ('open','in_progress')`).Scan(&totalRewardsLocked)
	database.DB.QueryRow(`SELECT COALESCE(SUM(reward_algo),0) FROM bounties WHERE status='completed'`).Scan(&totalRewardsPaid)

	c.JSON(200, models.APIResponse{Success: true, Data: gin.H{
		"total_bounties":      totalBounties,
		"open_bounties":       openBounties,
		"completed_bounties":  completedBounties,
		"total_users":         totalUsers,
		"total_rewards_locked": totalRewardsLocked,
		"total_rewards_paid":  totalRewardsPaid,
	}})
}

// GET /api/notifications — Get user notifications
func (h *LeaderboardHandler) GetNotifications(c *gin.Context) {
	pid, _ := c.Get("profile_id")
	rows, err := database.DB.Query(`
		SELECT id, type, title, message, bounty_id, is_read, created_at
		FROM notifications WHERE user_id = $1
		ORDER BY created_at DESC LIMIT 50
	`, pid)
	if err != nil {
		// Notifications table may not exist — return empty array gracefully
		c.JSON(200, models.APIResponse{Success: true, Data: []gin.H{}})
		return
	}
	defer rows.Close()

	var notifs []gin.H
	for rows.Next() {
		var id, ntype, title, message string
		var bountyID *string
		var isRead bool
		var createdAt interface{}
		rows.Scan(&id, &ntype, &title, &message, &bountyID, &isRead, &createdAt)
		notifs = append(notifs, gin.H{
			"id": id, "type": ntype, "title": title, "message": message,
			"bounty_id": bountyID, "is_read": isRead, "created_at": createdAt,
		})
	}
	if notifs == nil {
		notifs = []gin.H{}
	}
	c.JSON(200, models.APIResponse{Success: true, Data: notifs})
}

// PUT /api/notifications/read — Mark all notifications as read
func (h *LeaderboardHandler) MarkNotificationsRead(c *gin.Context) {
	pid, _ := c.Get("profile_id")
	database.DB.Exec(`UPDATE notifications SET is_read = TRUE WHERE user_id = $1`, pid)
	c.JSON(200, models.APIResponse{Success: true, Message: "Notifications marked as read"})
}
