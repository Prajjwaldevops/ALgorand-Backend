package middleware

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"bountyvault/internal/database"

	"github.com/clerk/clerk-sdk-go/v2/jwt"
	"github.com/gin-gonic/gin"
	gojwt "github.com/golang-jwt/jwt/v5"
)

// ClerkVerifyTokenMiddleware purely verifies the Clerk JWT and sets the subject.
// This is strictly for the Sync endpoint which creates the database profile.
func ClerkVerifyTokenMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Authorization header required"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := jwt.Verify(c.Request.Context(), &jwt.VerifyParams{
			Token:  tokenString,
			Leeway: 5 * time.Second,
		})
		if err != nil {
			log.Printf("[WARN] ClerkVerifyTokenMiddleware failed: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid session token: " + err.Error()})
			c.Abort()
			return
		}

		c.Set("clerk_id", claims.Subject)
		c.Next()
	}
}

// ClerkAuthMiddleware validates Clerk session JWTs and resolves user profile
func ClerkAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Authorization header required",
			})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid authorization format. Use 'Bearer <token>'",
			})
			c.Abort()
			return
		}

		// Verify the Clerk JWT
		claims, err := jwt.Verify(c.Request.Context(), &jwt.VerifyParams{
			Token:  tokenString,
			Leeway: 5 * time.Second,
		})
		if err != nil {
			log.Printf("[WARN] ClerkAuthMiddleware verification failed for route %s: %v", c.Request.URL.Path, err)
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid or expired session token: " + err.Error(),
			})
			c.Abort()
			return
		}

		clerkUserID := claims.Subject
		if clerkUserID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid token: missing subject",
			})
			c.Abort()
			return
		}

		// Map the Clerk user to our internal profiles table
		var profileID string
		var role string
		err = database.DB.QueryRow(`
			SELECT id, role FROM profiles WHERE clerk_id = $1
		`, clerkUserID).Scan(&profileID, &role)

		if err == sql.ErrNoRows {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "User profile not found. Please complete registration.",
			})
			c.Abort()
			return
		} else if err != nil {
			log.Printf("[ERROR] Database query failed in auth middleware: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Error resolving user profile",
			})
			c.Abort()
			return
		}

		// Set user context for the handlers
		c.Set("clerk_id", clerkUserID)
		c.Set("profile_id", profileID)
		c.Set("role", role)

		c.Next()
	}
}

// OptionalClerkAuth extracts user info if token present, but doesn't block
func OptionalClerkAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		claims, err := jwt.Verify(c.Request.Context(), &jwt.VerifyParams{
			Token: tokenString,
		})
		if err == nil && claims.Subject != "" {
			var profileID string
			var role string
			err = database.DB.QueryRow(`
				SELECT id, role FROM profiles WHERE clerk_id = $1
			`, claims.Subject).Scan(&profileID, &role)

			if err == nil {
				c.Set("clerk_id", claims.Subject)
				c.Set("profile_id", profileID)
				c.Set("role", role)
			}
		}

		c.Next()
	}
}

// AdminAuthMiddleware verifies admin JWT tokens (separate from Clerk)
// Uses ADMIN_JWT_SECRET to validate tokens issued by /api/admin/login
func AdminAuthMiddleware(adminJWTSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Admin authorization required"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid authorization format"})
			c.Abort()
			return
		}

		// Parse and validate the admin JWT
		token, err := gojwt.Parse(tokenString, func(token *gojwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*gojwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(adminJWTSecret), nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid or expired admin token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(gojwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid token claims"})
			c.Abort()
			return
		}

		role, _ := claims["role"].(string)
		if role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Admin role required"})
			c.Abort()
			return
		}

		c.Set("admin_id", claims["sub"])
		c.Set("admin_username", claims["username"])
		c.Next()
	}
}
