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

	// Initialize Clerk
	clerk.SetKey(cfg.ClerkSecretKey)

	// Connect DB
	if err := database.Connect(cfg.DatabaseURL); err != nil {
		log.Fatalf("[FATAL] Failed to connect to database: %v", err)
	}
	defer database.DB.Close()
	log.Println("[OK] Database connected")

	// Services
	algoSvc, err := services.NewAlgorandService(cfg)
	if err != nil {
		log.Printf("[WARN] Algorand service unavailable: %v", err)
		algoSvc = nil
	} else {
		log.Println("[OK] Algorand service connected")
	}

	ipfsSvc := services.NewIPFSService(cfg.PinataJWT, cfg.PinataGateway)
	log.Println("[OK] IPFS initialized")

	r2Svc, err := services.NewR2Service(cfg.R2AccountID, cfg.R2AccessKeyID, cfg.R2SecretAccessKey, cfg.R2BucketName, cfg.R2PublicURL)
	if err != nil {
		log.Printf("[WARN] R2 init failed: %v", err)
	} else {
		log.Println("[OK] R2 initialized")
	}

	// Handlers
	authHandler := handlers.NewAuthHandler(cfg)
	bountyHandler := handlers.NewBountyHandler(cfg, algoSvc, ipfsSvc, r2Svc)
	daoHandler := handlers.NewDAOHandler(cfg, algoSvc, ipfsSvc)
	leaderboardHandler := handlers.NewLeaderboardHandler()
	dashboardHandler := handlers.NewDashboardHandler()
	adminHandler := handlers.NewAdminHandler(cfg)

	// Gin setup
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// Middleware
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.RequestLogger())
	r.Use(middleware.RequestID())
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.MaxBodySize(10 << 20))
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

	// ✅ ROOT route (IMPORTANT for Railway)
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "running",
			"service": "bountyvault-api",
		})
	})

	// Health
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "healthy",
			"time":   time.Now().UTC(),
		})
	})

	// API
	api := r.Group("/api")

	authSync := api.Group("/auth")
	authSync.Use(middleware.ClerkVerifyTokenMiddleware())
	authSync.POST("/sync", authHandler.SyncProfile)

	authProtected := api.Group("/auth")
	authProtected.Use(middleware.ClerkAuthMiddleware())
	{
		authProtected.GET("/me", authHandler.GetMe)
		authProtected.PUT("/profile", authHandler.UpdateProfile)
		authProtected.PUT("/wallet", authHandler.LinkWallet)
		authProtected.PUT("/role", authHandler.SwitchRole)
	}

	bounties := api.Group("/bounties")
	{
		bounties.GET("", bountyHandler.ListBounties)
		bounties.GET("/:id", bountyHandler.GetBounty)
	}

	bProtected := api.Group("/bounties")
	bProtected.Use(middleware.ClerkAuthMiddleware())
	{
		bProtected.POST("", bountyHandler.CreateBounty)
	}

	api.GET("/categories", bountyHandler.ListCategories)

	dao := api.Group("/dao")
	{
		dao.GET("/disputes", daoHandler.ListActiveDisputes)
	}

	daoProtected := api.Group("/dao")
	daoProtected.Use(middleware.ClerkAuthMiddleware())
	{
		daoProtected.POST("/disputes/:id/vote", daoHandler.CastVote)
	}

	api.GET("/leaderboard", leaderboardHandler.GetLeaderboard)

	dash := api.Group("/dashboard")
	dash.Use(middleware.ClerkAuthMiddleware())
	{
		dash.GET("/stats", dashboardHandler.GetUserStats)
	}

	adminAPI := api.Group("/admin")
	adminAPI.POST("/login", adminHandler.Login)

	// ✅ FIXED PORT LOGIC (CRITICAL)
	port := os.Getenv("PORT")
	if port == "" {
		port = cfg.Port
	}
	if port == "" {
		port = "5000"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	go func() {
		log.Printf("[OK] Server running on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Server failed: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[INFO] Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("[FATAL] Shutdown failed: %v", err)
	}

	log.Println("[OK] Server exited")
}
