package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bountyvault/internal/config"
	"bountyvault/internal/database"
	"bountyvault/internal/handlers"
	"bountyvault/internal/middleware"
	"bountyvault/internal/services"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("[FATAL] Failed to load config: %v", err)
	}

	// Initialize Clerk Global SDK Key
	clerk.SetKey(cfg.ClerkSecretKey)

	// Connect to database
	if err := database.Connect(cfg.DatabaseURL); err != nil {
		log.Fatalf("[FATAL] Failed to connect to database: %v", err)
	}
	defer database.DB.Close()
	log.Println("[OK] Database connected")

	// Initialize services
	algoSvc, err := services.NewAlgorandService(cfg)
	if err != nil {
		log.Printf("[WARN] Algorand service unavailable: %v", err)
		algoSvc = nil
	} else {
		log.Println("[OK] Algorand service connected")
	}

	ipfsSvc := services.NewIPFSService(cfg.PinataJWT, cfg.PinataGateway)
	log.Println("[OK] IPFS/Pinata service initialized")

	r2Svc, err := services.NewR2Service(cfg.R2AccountID, cfg.R2AccessKeyID, cfg.R2SecretAccessKey, cfg.R2BucketName, cfg.R2PublicURL)
	if err != nil {
		log.Printf("[WARN] R2 service failed to initialize: %v", err)
	} else {
		log.Println("[OK] Cloudflare R2 service initialized")
	}

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(cfg)
	bountyHandler := handlers.NewBountyHandler(cfg, algoSvc, ipfsSvc, r2Svc)
	daoHandler := handlers.NewDAOHandler(cfg, algoSvc, ipfsSvc)
	leaderboardHandler := handlers.NewLeaderboardHandler()
	dashboardHandler := handlers.NewDashboardHandler()
	adminHandler := handlers.NewAdminHandler(cfg)

	// Setup Gin
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// ========================
	// Global Middleware Stack
	// ========================
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.RequestLogger())
	r.Use(middleware.RequestID())
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.MaxBodySize(10 << 20)) // 10MB max
	r.Use(middleware.RateLimitMiddleware(cfg.RateLimitRPS, cfg.RateLimitBurst))

	// CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Request-ID"},
		ExposeHeaders:    []string{"Content-Length", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// ========================
	// Health Check
	// ========================
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "healthy",
			"service": "bountyvault-api",
			"version": "3.1.0",
			"time":    time.Now().UTC(),
		})
	})

	// ========================
	// API Routes (v3.1)
	// ========================
	api := r.Group("/api")

	// --- Auth Sync (Profile Initialization — Clerk token only, no profile needed) ---
	authSync := api.Group("/auth")
	authSync.Use(middleware.ClerkVerifyTokenMiddleware())
	authSync.POST("/sync", authHandler.SyncProfile)

	// --- Auth (protected — requires profile in DB) ---
	authProtected := api.Group("/auth")
	authProtected.Use(middleware.ClerkAuthMiddleware())
	{
		authProtected.GET("/me", authHandler.GetMe)
		authProtected.PUT("/profile", authHandler.UpdateProfile)
		authProtected.PUT("/wallet", authHandler.LinkWallet)
		authProtected.PUT("/role", authHandler.SwitchRole) // v3.1: role switching
	}

	// --- Bounties (public read) ---
	bounties := api.Group("/bounties")
	{
		bounties.GET("", bountyHandler.ListBounties)
		bounties.GET("/:id", bountyHandler.GetBounty)
	}

	// --- Bounties (protected write) ---
	bProtected := api.Group("/bounties")
	bProtected.Use(middleware.ClerkAuthMiddleware())
	{
		bProtected.POST("", bountyHandler.CreateBounty)
		bProtected.POST("/:id/lock", bountyHandler.LockBounty)
		bProtected.POST("/:id/confirm-lock", bountyHandler.ConfirmLock)
		bProtected.POST("/:id/submit", bountyHandler.SubmitWork)
		bProtected.PUT("/:id/approve", bountyHandler.ApproveSubmission)
		bProtected.PUT("/:id/reject", bountyHandler.RejectSubmission)
		bProtected.POST("/:id/dispute", bountyHandler.InitiateDispute)
		bProtected.POST("/:id/letgo", bountyHandler.LetGoBounty)           // v3.1: freelancer let-go
		bProtected.POST("/:id/cancel", bountyHandler.CancelBounty)         // v3.1: creator cancel
		bProtected.POST("/:id/refund-expired", bountyHandler.RefundExpired) // v3.1: permissionless
		bProtected.POST("/:id/build-approve-payout", bountyHandler.BuildApprovePayout)
		bProtected.POST("/:id/rate", bountyHandler.RateWorker)
		bProtected.GET("/:id/submissions", bountyHandler.ListSubmissions)
		// v3.2: Bounty Acceptance Flow
		bProtected.POST("/:id/accept", bountyHandler.AcceptBounty)
		bProtected.GET("/:id/acceptances", bountyHandler.GetAcceptances)
		bProtected.PUT("/:id/review-acceptance", bountyHandler.ReviewAcceptance)
		bProtected.POST("/:id/confirm-acceptance", bountyHandler.ConfirmAcceptance)
		bProtected.GET("/:id/my-acceptance", bountyHandler.GetMyAcceptanceStatus)
	}


	// --- Categories (public) ---
	api.GET("/categories", bountyHandler.ListCategories)

	// --- DAO Voting (v3.1 + v3.6 voting compliance) ---
	dao := api.Group("/dao")
	{
		dao.GET("/disputes", daoHandler.ListActiveDisputes)
		dao.GET("/disputes/:id", daoHandler.GetDisputeDetail)
		dao.GET("/disputes/:id/votes", daoHandler.GetDisputeVotes)
		dao.GET("/dispute-debug", func(c *gin.Context) {
			rows, err := database.DB.Query("SELECT id, dispute_id, bounty_id, status FROM disputes LIMIT 20")
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			defer rows.Close()
			var out []map[string]interface{}
			for rows.Next() {
				var id, did, bid, status string
				rows.Scan(&id, &did, &bid, &status)
				out = append(out, map[string]interface{}{"id": id, "dispute_id": did, "bounty_id": bid, "status": status})
			}
			c.JSON(200, out)
		})
	}
	daoProtected := api.Group("/dao")
	daoProtected.Use(middleware.ClerkAuthMiddleware())
	{
		daoProtected.POST("/disputes/:id/build-vote-txn", daoHandler.BuildVoteTxn)       // v3.6: build Payment+AppCall group
		daoProtected.POST("/disputes/:id/vote", daoHandler.CastVote)                     // v3.1: submit signed vote txns
		daoProtected.POST("/disputes/:id/build-finalize-txn", daoHandler.BuildFinalizeTxn) // v3.6: build resolve_dao_dispute()
		daoProtected.POST("/disputes/:id/finalize", daoHandler.FinalizeVote)              // v3.1: submit signed finalize txn
		daoProtected.GET("/voting-status", daoHandler.GetVotingStatus)                    // v3.6: voting compliance
	}

	// --- Leaderboard & Stats (public) ---
	api.GET("/leaderboard", leaderboardHandler.GetLeaderboard)
	api.GET("/leaderboard/top-creators", leaderboardHandler.GetTopCreators)
	api.GET("/stats", leaderboardHandler.GetPlatformStats)

	// --- Notifications (protected) ---
	notifs := api.Group("/notifications")
	notifs.Use(middleware.ClerkAuthMiddleware())
	{
		notifs.GET("", leaderboardHandler.GetNotifications)
		notifs.PUT("/read", leaderboardHandler.MarkNotificationsRead)
	}

	// --- Dashboard (protected, user-specific) ---
	dash := api.Group("/dashboard")
	dash.Use(middleware.ClerkAuthMiddleware())
	{
		dash.GET("/stats", dashboardHandler.GetUserStats)
		dash.GET("/my-bounties", dashboardHandler.GetMyBounties)
		dash.GET("/my-submissions", dashboardHandler.GetMySubmissions)
		dash.GET("/working-bounties", dashboardHandler.GetWorkingBounties)
		dash.GET("/disputes", dashboardHandler.GetMyDisputes)
		// v3.2: Acceptance dashboard views
		dash.GET("/pending-acceptances", dashboardHandler.GetPendingAcceptances)
		dash.GET("/my-acceptances", dashboardHandler.GetMyAcceptances)
		// v3.3: Transaction log
		dash.GET("/transactions", dashboardHandler.GetTransactionLog)
	}

	// --- Bounty Status (protected) ---
	bStatusProtected := api.Group("/bounties")
	bStatusProtected.Use(middleware.ClerkAuthMiddleware())
	{
		bStatusProtected.GET("/:id/status-history", dashboardHandler.GetBountyStatusHistory)
		bStatusProtected.POST("/:id/status-update", dashboardHandler.UpdateBountyStatus)
	}

	// ========================
	// Admin Panel Routes (v3.1 — separate JWT auth)
	// ========================
	adminAPI := api.Group("/admin")
	{
		adminAPI.POST("/login", adminHandler.Login)
	}
	adminProtected := adminAPI.Group("")
	adminProtected.Use(middleware.AdminAuthMiddleware(cfg.AdminJWTSecret))
	{
		adminProtected.GET("/stats", adminHandler.GetStats)
		adminProtected.GET("/users", adminHandler.ListUsers)
		adminProtected.GET("/bounties", adminHandler.ListAllBounties)
		adminProtected.GET("/transactions", adminHandler.ListTransactions)
		adminProtected.GET("/disputes", adminHandler.ListAllDisputes)
		adminProtected.GET("/audit-log", adminHandler.GetAuditLog)
	}

	// ========================
	// Server with Graceful Shutdown
	// ========================
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("[OK] BountyVault API v3.1 listening on :%s (%s)", cfg.Port, cfg.Environment)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[INFO] Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("[FATAL] Server forced shutdown: %v", err)
	}

	log.Println("[OK] Server exited gracefully")
}
