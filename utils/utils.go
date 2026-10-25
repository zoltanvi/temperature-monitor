package utils

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"tempMonitor/persistence"
)

const (
	EnvDatabasePath     = "DATABASE_PATH"
	EnvPort             = "PORT"
	EnvFlushIntervalSec = "FLUSH_INTERVAL_SEC"

	DefaultFlushIntervalSec int64 = 1
)

func ReadEnvVar(environmentVariable string) string {
	dbPath := os.Getenv(environmentVariable)
	if dbPath == "" {
		log.Fatalf("Environment variable %v was not provided!", environmentVariable)
	}

	return dbPath
}

// ReadFlushIntervalSec returns FLUSH_INTERVAL_SEC or DefaultFlushIntervalSec.
// Values < 1 are fatal.
func ReadFlushIntervalSec() int64 {
	raw := os.Getenv(EnvFlushIntervalSec)
	if raw == "" {
		return DefaultFlushIntervalSec
	}

	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		log.Fatalf("Environment variable %s=%q is not a valid integer: %v", EnvFlushIntervalSec, raw, err)
	}
	if v < 1 {
		log.Fatalf("Environment variable %s must be >= 1, got %d", EnvFlushIntervalSec, v)
	}
	return v
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
