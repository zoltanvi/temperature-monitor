package utils

import (
	"log"
	"os"
	"path/filepath"
	"tempMonitor/persistence"
)

const (
	EnvDatabasePath = "DATABASE_PATH"
	EnvPort         = "PORT"
)

func ReadEnvVar(environmentVariable string) string {
	dbPath := os.Getenv(environmentVariable)
	if dbPath == "" {
		log.Fatalf("Environment variable %v was not provided!", environmentVariable)
	}

	return dbPath
}

func CreateDbPathIfNeeded(dbPath string) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("Failed to create database directory: %v, error: %v", dir, err)
	}
}

func OpenStore(dbPath string) *persistence.Store {
	store, err := persistence.OpenDatabase(dbPath)
	if err != nil {
		log.Fatal(err)
	}

	return store
}
