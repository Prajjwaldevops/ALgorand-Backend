//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Println("No .env file found, relying on environment variables")
	}

	dbUrl := os.Getenv("DATABASE_URL")
	if dbUrl == "" {
		log.Fatal("DATABASE_URL must be set")
	}

	db, err := sql.Open("postgres", dbUrl)
	if err != nil {
		log.Fatalf("Error opening database: %v\n", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Error connecting to the database: %v\n", err)
	}

	migrationContext, err := ioutil.ReadFile("../database/migration-v3.3-lifecycle.sql")
	if err != nil {
		log.Fatalf("Error reading migration file: %v\n", err)
	}

	fmt.Println("Running migration...")
	_, err = db.Exec(string(migrationContext))
	if err != nil {
		log.Fatalf("Error running migration: %v\n", err)
	}

	fmt.Println("Migration successful!")
}
