//go:build ignore

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
	godotenv.Load(".env")
	dbUrl := os.Getenv("DATABASE_URL")
	db, err := sql.Open("postgres", dbUrl)
	if err != nil { log.Fatal(err) }

	rows, err := db.Query(`SELECT id, clerk_id, username, email FROM profiles`)
	if err != nil { log.Fatal(err) }
	for rows.Next() {
		var id, clerk, user, email string
		rows.Scan(&id, &clerk, &user, &email)
		fmt.Printf("Profile: %s | clerk: %s | u: %s | e: %s\n", id, clerk, user, email)
	}
}
