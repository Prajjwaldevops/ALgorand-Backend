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

	// Check table schema - what columns does transaction_log actually have?
	fmt.Println("=== transaction_log columns ===")
	rows, err := db.Query(`
		SELECT column_name, data_type, udt_name, is_nullable
		FROM information_schema.columns 
		WHERE table_name = 'transaction_log'
		ORDER BY ordinal_position
	`)
	if err != nil {
		log.Fatal("Schema query failed:", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, dtype, udt, nullable string
		rows.Scan(&name, &dtype, &udt, &nullable)
		fmt.Printf("  %-25s %-20s %-20s nullable=%s\n", name, dtype, udt, nullable)
	}

	// Show all bounties
	fmt.Println("\n=== All bounties ===")
	rows2, _ := db.Query(`
		SELECT id, title, status, 
		       COALESCE(accepted_freelancer_id::text, 'none'),
		       COALESCE(escrow_txn_id, 'none'),
		       COALESCE(app_id::text, 'none'),
		       creator_id,
		       reward_algo
		FROM bounties ORDER BY created_at DESC
	`)
	if rows2 != nil {
		defer rows2.Close()
		for rows2.Next() {
			var id, title, status, accFL, escrowTxn, appID, creatorID string
			var reward float64
			rows2.Scan(&id, &title, &status, &accFL, &escrowTxn, &appID, &creatorID, &reward)
			fmt.Printf("  ID: %s\n    Title: %s\n    Status: %s | Reward: %.2f ALGO\n    AcceptedFL: %s\n    EscrowTxn: %s\n    AppID: %s | Creator: %s\n\n",
				id, title, status, reward, accFL, escrowTxn, appID, creatorID)
		}
	}

	// Try a manual insert into transaction_log to see if it works
	fmt.Println("\n=== Testing INSERT into transaction_log ===")
	
	// First get a bounty ID and creator ID
	var testBountyID, testCreatorID string
	var testReward float64
	var testEscrowTxn *string
	err = db.QueryRow(`
		SELECT id, creator_id, reward_algo, escrow_txn_id 
		FROM bounties 
		WHERE escrow_txn_id IS NOT NULL 
		LIMIT 1
	`).Scan(&testBountyID, &testCreatorID, &testReward, &testEscrowTxn)
	if err != nil {
		fmt.Printf("  No bounty with escrow_txn_id found: %v\n", err)
		// Try any bounty
		err = db.QueryRow(`SELECT id, creator_id, reward_algo FROM bounties LIMIT 1`).Scan(&testBountyID, &testCreatorID, &testReward)
		if err != nil {
			fmt.Printf("  No bounties found at all: %v\n", err)
			return
		}
		fmt.Printf("  Using bounty %s (no escrow txn)\n", testBountyID)
	} else {
		fmt.Printf("  Found bounty with escrow: %s, txn=%s\n", testBountyID, *testEscrowTxn)
	}

	// Try inserting
	txnID := "test_diagnostic_txn_001"
	if testEscrowTxn != nil {
		txnID = *testEscrowTxn
	}
	
	_, insertErr := db.Exec(`
		INSERT INTO transaction_log (bounty_id, actor_id, event, txn_id, txn_note, amount_algo)
		VALUES ($1, $2, 'escrow_locked', $3, $4, $5)
	`, testBountyID, testCreatorID, txnID, "BountyVault:diagnostic_test", testReward)
	
	if insertErr != nil {
		fmt.Printf("  INSERT FAILED: %v\n", insertErr)
	} else {
		fmt.Println("  INSERT SUCCEEDED!")
		// Verify
		var newCount int
		db.QueryRow("SELECT COUNT(*) FROM transaction_log").Scan(&newCount)
		fmt.Printf("  transaction_log now has %d rows\n", newCount)
	}

	// Check bounty_acceptances
	fmt.Println("\n=== bounty_acceptances ===")
	rows3, _ := db.Query(`
		SELECT a.id, a.bounty_id, a.freelancer_id, a.status, b.title
		FROM bounty_acceptances a
		JOIN bounties b ON a.bounty_id = b.id
		ORDER BY a.created_at DESC
	`)
	if rows3 != nil {
		defer rows3.Close()
		for rows3.Next() {
			var aID, aBountyID, aFreelancerID, aStatus, bTitle string
			rows3.Scan(&aID, &aBountyID, &aFreelancerID, &aStatus, &bTitle)
			fmt.Printf("  Acceptance: %s | Bounty: %s | FL: %s | Status: %s | Title: %s\n",
				aID[:8], aBountyID[:8], aFreelancerID[:8], aStatus, bTitle)
		}
	}
}
