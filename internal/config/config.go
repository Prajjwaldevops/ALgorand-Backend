package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	// Server
	Port        string
	Environment string

	// Database
	DatabaseURL        string
	SupabaseURL        string
	SupabaseAnonKey    string
	SupabaseServiceKey string

	// Clerk Auth
	ClerkSecretKey string

	// Algorand
	AlgoNodeURL    string
	AlgoIndexerURL string
	AlgoNetwork    string
	BountyAppID    uint64

	// Pinata / IPFS — used ONLY for transaction metadata JSON
	PinataJWT     string
	PinataGateway string

	// Cloudflare R2 — used for submission files and avatars
	R2AccountID       string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2BucketName      string
	R2PublicURL       string

	// Admin Panel Authentication (separate from Clerk)
	AdminUsername  string
	AdminPassword  string
	AdminJWTSecret string

	// CORS
	AllowedOrigins []string

	// Rate Limiting
	RateLimitRPS   int
	RateLimitBurst int

	// Temporary Escrow Bypass
	EscrowMnemonic string
	EscrowAddress  string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	err := godotenv.Load()
	if err != nil {
		log.Println("[INFO] No .env file found, using environment variables")
	}

	appID, _ := strconv.ParseUint(getEnv("BOUNTY_APP_ID", "0"), 10, 64)
	rps, _ := strconv.Atoi(getEnv("RATE_LIMIT_RPS", "10"))
	burst, _ := strconv.Atoi(getEnv("RATE_LIMIT_BURST", "20"))

	originsStr := getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")
	origins := strings.Split(originsStr, ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
	}

	cfg := &Config{
		Port:               getEnv("PORT", "8080"),
		Environment:        getEnv("ENVIRONMENT", "development"),
		DatabaseURL:        mustGetEnv("DATABASE_URL"),
		SupabaseURL:        getEnv("SUPABASE_URL", ""),
		SupabaseAnonKey:    getEnv("SUPABASE_ANON_KEY", ""),
		SupabaseServiceKey: getEnv("SUPABASE_SERVICE_KEY", ""),
		ClerkSecretKey:     getEnv("CLERK_SECRET_KEY", ""),
		AlgoNodeURL:        getEnv("ALGO_NODE_URL", "https://testnet-api.algonode.cloud"),
		AlgoIndexerURL:     getEnv("ALGO_INDEXER_URL", "https://testnet-idx.algonode.cloud"),
		AlgoNetwork:        getEnv("ALGO_NETWORK", "testnet"),
		BountyAppID:        appID,
		PinataJWT:          getEnv("PINATA_JWT", ""),
		PinataGateway:      getEnv("PINATA_GATEWAY_URL", "https://gateway.pinata.cloud/ipfs"),
		R2AccountID:        getEnv("R2_ACCOUNT_ID", ""),
		R2AccessKeyID:      getEnv("R2_ACCESS_KEY_ID", ""),
		R2SecretAccessKey:  getEnv("R2_SECRET_ACCESS_KEY", ""),
		R2BucketName:       getEnv("R2_BUCKET_NAME", "bountyvault-files"),
		R2PublicURL:        getEnv("R2_PUBLIC_URL", ""),
		AdminUsername:      getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword:      getEnv("ADMIN_PASSWORD", "admin"),
		AdminJWTSecret:     getEnv("ADMIN_JWT_SECRET", "bountyvault-admin-secret"),
		AllowedOrigins:     origins,
		RateLimitRPS:       rps,
		RateLimitBurst:     burst,
		EscrowMnemonic:     getEnv("ESCROW_MNEMONIC", ""),
		EscrowAddress:      getEnv("ESCROW_ADDRESS", ""),
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("[FATAL] Required environment variable %s is not set", key)
	}
	return val
}
