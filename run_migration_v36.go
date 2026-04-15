package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer db.Close()

	statements := []string{
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS last_dao_vote_at TIMESTAMPTZ`,
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS is_dao_banned BOOLEAN DEFAULT FALSE`,
	}

	for _, stmt := range statements {
		_, err := db.Exec(stmt)
		if err != nil {
			log.Printf("Warning executing '%s': %v", stmt, err)
		} else {
			fmt.Printf("OK: %s\n", stmt)
		}
	}

	fmt.Println("Migration v3.6 complete!")
}
