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
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// List ALL enum types and their values
	rows, _ := db.Query(`
		SELECT t.typname, e.enumlabel, e.enumsortorder
		FROM pg_type t
		JOIN pg_enum e ON t.oid = e.enumtypid
		WHERE t.typname LIKE '%vote%' OR t.typname LIKE '%choice%'
		ORDER BY t.typname, e.enumsortorder
	`)
	defer rows.Close()
	fmt.Println("=== All vote-related enums ===")
	for rows.Next() {
		var name, label string
		var order float64
		rows.Scan(&name, &label, &order)
		fmt.Printf("  enum: %-25s value: %s\n", name, label)
	}

	// Also check the actual column type
	var colType string
	db.QueryRow(`
		SELECT udt_name FROM information_schema.columns 
		WHERE table_name = 'dao_votes' AND column_name = 'vote'
	`).Scan(&colType)
	fmt.Printf("\ndao_votes.vote column udt_name: %s\n", colType)
}
