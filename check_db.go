package main

import (
	"bountyvault/internal/config"
	"bountyvault/internal/database"
	"fmt"
	"log"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	if err := database.Connect(cfg.DatabaseURL); err != nil {
		log.Fatalf("db error: %v", err)
	}
	
	rows, err := database.DB.Query("SELECT id, clerk_id, username, role, total_earned_algo, display_name FROM profiles")
	if err != nil {
		log.Fatalf("query error: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, clerkID, username, role string
		var totalEarnedAlgo float64
		var dname *string
		err := rows.Scan(&id, &clerkID, &username, &role, &totalEarnedAlgo, &dname)
		fmt.Printf("id=%s clerk=%s user=%s role=%s earned=%f err=%v\n", id, clerkID, username, role, totalEarnedAlgo, err)
	}
}
