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
	godotenv.Load("../.env")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer db.Close()

	// Step 1: Make 'action' column nullable
	fmt.Println("Step 1: ALTER action column to DROP NOT NULL...")
	_, err = db.Exec(`ALTER TABLE transaction_log ALTER COLUMN action DROP NOT NULL`)
	if err != nil {
		log.Fatal("ALTER failed:", err)
	}
	fmt.Println("  ✓ action column is now nullable")

	// Step 2: Backfill existing bounties that have escrow_txn_id with transaction_log entries
	fmt.Println("\nStep 2: Backfilling transaction_log from existing bounties...")
	rows, err := db.Query(`
		SELECT b.id, b.creator_id, b.accepted_freelancer_id, b.escrow_txn_id, b.reward_algo, 
		       COALESCE(b.app_id, 0)
		FROM bounties b
		WHERE b.escrow_txn_id IS NOT NULL 
		  AND b.accepted_freelancer_id IS NOT NULL
	`)
	if err != nil {
		log.Fatal("Bounty query failed:", err)
	}
	defer rows.Close()

	inserted := 0
	for rows.Next() {
		var bountyID, creatorID, freelancerID, escrowTxn string
		var reward float64
		var appID int64
		rows.Scan(&bountyID, &creatorID, &freelancerID, &escrowTxn, &reward, &appID)

		// Insert escrow_locked for creator
		_, err := db.Exec(`
			INSERT INTO transaction_log (bounty_id, actor_id, event, txn_id, txn_note, amount_algo)
			VALUES ($1, $2, 'escrow_locked', $3, $4, $5)
		`, bountyID, creatorID, escrowTxn, fmt.Sprintf("BountyVault:escrow_locked:%d", appID), reward)
		if err != nil {
			fmt.Printf("  ✗ escrow_locked insert failed for bounty %s: %v\n", bountyID[:8], err)
		} else {
			fmt.Printf("  ✓ escrow_locked for bounty %s (creator %s)\n", bountyID[:8], creatorID[:8])
			inserted++
		}

		// Insert bounty_accepted for freelancer
		_, err = db.Exec(`
			INSERT INTO transaction_log (bounty_id, actor_id, event, txn_id, txn_note, amount_algo)
			VALUES ($1, $2, 'bounty_accepted', $3, $4, $5)
		`, bountyID, freelancerID, escrowTxn, fmt.Sprintf("BountyVault:bounty_accepted:%d", appID), reward)
		if err != nil {
			fmt.Printf("  ✗ bounty_accepted insert failed for bounty %s: %v\n", bountyID[:8], err)
		} else {
			fmt.Printf("  ✓ bounty_accepted for bounty %s (freelancer %s)\n", bountyID[:8], freelancerID[:8])
			inserted++
		}
	}

	fmt.Printf("\n  Total inserted: %d entries\n", inserted)

	// Verify
	var count int
	db.QueryRow("SELECT COUNT(*) FROM transaction_log").Scan(&count)
	fmt.Printf("\n  transaction_log now has %d rows ✓\n", count)
}
