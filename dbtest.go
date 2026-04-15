//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

func main() {
	dbURL := "postgresql://postgres.fnjdtoishdxfgzpbmsqy:2FTjHBCmlVdP11zM@aws-1-ap-southeast-2.pooler.supabase.com:6543/postgres"
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var newID string
	err = db.QueryRow(`
		INSERT INTO profiles (clerk_id, username, display_name, role, email)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, "test_clerk_id", "test_user", "test_display", "freelancer", "test@test.com").Scan(&newID)

	if err != nil {
		fmt.Printf("DB INSERT ERROR: %v\n", err)
	} else {
		fmt.Printf("DB INSERT SUCCESS! ID: %s\n", newID)
	}
}
