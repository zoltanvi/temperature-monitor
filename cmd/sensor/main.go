package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"tempMonitor/sensor"
	"tempMonitor/utils"
	"time"
)

// Required environment variables:
//
// DATABASE_PATH:  "/mnt/hdd/temperatures.db"
func main() {
	log.Println("Starting Sensor...")

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	dbPath := utils.ReadEnvVar(utils.EnvDatabasePath)
	log.Printf("Using database: %s", dbPath)

	utils.CreateDbPathIfNeeded(dbPath)

	store := utils.OpenStore(dbPath)
	defer store.Close()

	if err := store.EnsureRollupsReady(); err != nil {
		log.Printf("WARNING: rollup backfill failed; API will use raw until next successful backfill: %v", err)
	}

	interval := time.Second
	log.Printf("Sensor is running. Storing temperatures in interval: %v", interval)

	sensor := sensor.New(interval, func(temp float64) {
		if err := store.Insert(temp); err != nil {
			log.Printf("Database insert error at UTC time [%s]: %v", time.Now().UTC(), err)
		}
	})

	sensor.Start()

	<-ctx.Done()

	log.Println("Sensor shutting down...")

	sensor.Stop()

	log.Println("Sensor shutdown complete.")
}
