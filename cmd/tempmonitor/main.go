package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"tempMonitor/buffer"
	"tempMonitor/downsampling"
	"tempMonitor/merge"
	"tempMonitor/models"
	"tempMonitor/persistence"
	"tempMonitor/sensor"
	"tempMonitor/statistics"
	"tempMonitor/utils"
	"time"
)

// Environment variables:
//
// DATABASE_PATH:       "/mnt/hdd/temperatures.db"   (required)
// PORT:                "5500"                       (required)
// FLUSH_INTERVAL_SEC:  "3600"                       (optional, default 1)
func main() {
	log.Println("Starting temperature monitor...")

	startedAt := time.Now()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	dbPath := utils.ReadEnvVar(utils.EnvDatabasePath)
	port := utils.ReadEnvVar(utils.EnvPort)
	flushIntervalSec := utils.ReadFlushIntervalSec()

	log.Printf("Using database: %s", dbPath)
	log.Printf("Flush interval: %ds", flushIntervalSec)

	utils.CreateDbPathIfNeeded(dbPath)

	store := utils.OpenStore(dbPath)
	defer store.Close()

	buf := buffer.New()
	var flushMu sync.Mutex

	flush := func() {
		flushMu.Lock()
		defer flushMu.Unlock()

		pending := buf.SnapshotAll()
		if len(pending) == 0 {
			return
		}

		if err := store.InsertBatch(pending); err != nil {
			log.Printf("Flush failed (%d samples): %v", len(pending), err)
			buf.SetFlushResult(err)
			return
		}

		buf.RemoveThroughTimestamp(pending[len(pending)-1].Timestamp)
		buf.SetFlushResult(nil)
		log.Printf("Flushed %d samples to database", len(pending))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/temperature", func(w http.ResponseWriter, r *http.Request) {
		temperatureHandler(w, r, store, buf)
	})
	mux.HandleFunc("/api/v3/stats", func(w http.ResponseWriter, r *http.Request) {
		statsHandler(w, r, store, buf)
	})
	mux.HandleFunc("/api/v3/health", func(w http.ResponseWriter, r *http.Request) {
		healthHandler(w, r, store, buf, startedAt)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.FileServer(http.Dir(".")).ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, "index.html")
	})

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		log.Println("Listening on :" + port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	flushCtx, flushCancel := context.WithCancel(context.Background())

	var bg sync.WaitGroup
	bg.Add(1)
	go func() {
		defer bg.Done()

		if err := store.EnsureRollupsReady(); err != nil {
			log.Printf("WARNING: rollup backfill failed; API will use raw until next successful backfill: %v", err)
		}

		if ctx.Err() != nil {
			return
		}

		go runFlusher(flushCtx, time.Duration(flushIntervalSec)*time.Second, flush)

		interval := time.Second
		log.Printf("Sensor started (sample interval %v)", interval)
		s := sensor.New(interval, func(temp float64) {
			buf.Append(time.Now().Unix(), temp)
		})
		s.Start()
		defer s.Stop()

		<-ctx.Done()
	}()

	<-ctx.Done()

	log.Println("Shutting down...")
	flushCancel()
	bg.Wait()
	flush()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)

	log.Println("Shutdown complete.")
}

func runFlusher(ctx context.Context, interval time.Duration, flush func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			flush()
		}
	}
}

func temperatureHandler(w http.ResponseWriter, r *http.Request, store *persistence.Store, buf *buffer.Buffer) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	timeStart := time.Now()

	const paramFrom = "from"
	const paramTo = "to"
	const paramDownsampleTo = "downsampleTo"
	const paramWarningThreshold = "warningThreshold"
	const paramCriticalThreshold = "criticalThreshold"

	q := r.URL.Query()

	from, err := getInt64Param(q, paramFrom)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	to, err := getInt64Param(q, paramTo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	downsampleTo, err := getInt64Param(q, paramDownsampleTo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	warningThreshold, err := getFloatParam(q, paramWarningThreshold)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	criticalThreshold, err := getFloatParam(q, paramCriticalThreshold)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if from > to {
		http.Error(w, fmt.Sprintf("Parameter [%v] can't be later than [%v].", paramFrom, paramTo), http.StatusBadRequest)
		return
	}

	if warningThreshold > criticalThreshold {
		http.Error(w, fmt.Sprintf("Parameter [%v] can't be greater than [%v].", paramWarningThreshold, paramCriticalThreshold), http.StatusBadRequest)
		return
	}

	if downsampleTo > math.MaxInt32 || downsampleTo < math.MinInt32 {
		http.Error(w, fmt.Sprintf("Parameter [%v] is out of int range.", paramDownsampleTo), http.StatusBadRequest)
		return
	}

	lastTemp, hasLast, err := latestSample(store, buf)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error during reading the latest temperature: %v", err), http.StatusInternalServerError)
		return
	}

	meta, err := buildResponseMeta(store, hasLast, lastTemp)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error building response meta: %v", err), http.StatusInternalServerError)
		return
	}

	if !hasLast {
		meta.NoData = true
		meta.DataSource = "raw"
		meta.ProcessedSamples = 0

		stats := models.Statistics{}
		stats.RequestElapsedTime = fmt.Sprintf("%s", time.Since(timeStart))
		stats.RequestProcessedSamples = "0"

		writeJSON(w, &models.TemperatureApiResponse{
			AggregatedData: []models.AggregatedTempPoint{},
			Statistics:     stats,
			Meta:           meta,
		})
		return
	}

	aggregatedData, stats, processed, source, err := loadTemperatureData(
		store, buf, from, to, int(downsampleTo), lastTemp, warningThreshold, criticalThreshold)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error during reading temperatures: %v", err), http.StatusInternalServerError)
		return
	}

	meta.DataSource = source.String()
	meta.ProcessedSamples = processed
	meta.NoData = false

	stats.RequestElapsedTime = fmt.Sprintf("%s", time.Since(timeStart))
	stats.RequestProcessedSamples = fmt.Sprintf("%v", processed)

	writeJSON(w, &models.TemperatureApiResponse{
		AggregatedData: aggregatedData,
		Statistics:     stats,
		Meta:           meta,
	})
}

func statsHandler(w http.ResponseWriter, r *http.Request, store *persistence.Store, buf *buffer.Buffer) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	timeStart := time.Now()

	const paramFrom = "from"
	const paramTo = "to"
	const paramDownsampleTo = "downsampleTo"
	const paramWarningThreshold = "warningThreshold"
	const paramCriticalThreshold = "criticalThreshold"

	q := r.URL.Query()

	from, err := getInt64Param(q, paramFrom)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	to, err := getInt64Param(q, paramTo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	warningThreshold, err := getFloatParam(q, paramWarningThreshold)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	criticalThreshold, err := getFloatParam(q, paramCriticalThreshold)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if from > to {
		http.Error(w, fmt.Sprintf("Parameter [%v] can't be later than [%v].", paramFrom, paramTo), http.StatusBadRequest)
		return
	}

	if warningThreshold > criticalThreshold {
		http.Error(w, fmt.Sprintf("Parameter [%v] can't be greater than [%v].", paramWarningThreshold, paramCriticalThreshold), http.StatusBadRequest)
		return
	}

	downsampleTo := int64(1500)
	if q.Get(paramDownsampleTo) != "" {
		downsampleTo, err = getInt64Param(q, paramDownsampleTo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	lastTemp, hasLast, err := latestSample(store, buf)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error during reading the latest temperature: %v", err), http.StatusInternalServerError)
		return
	}

	if !hasLast {
		stats := models.Statistics{}
		stats.RequestElapsedTime = fmt.Sprintf("%s", time.Since(timeStart))
		stats.RequestProcessedSamples = "0"
		writeJSON(w, stats)
		return
	}

	_, stats, processed, _, err := loadTemperatureData(
		store, buf, from, to, int(downsampleTo), lastTemp, warningThreshold, criticalThreshold)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error during reading temperatures: %v", err), http.StatusInternalServerError)
		return
	}

	stats.RequestElapsedTime = fmt.Sprintf("%s", time.Since(timeStart))
	stats.RequestProcessedSamples = fmt.Sprintf("%v", processed)

	writeJSON(w, stats)
}

func healthHandler(w http.ResponseWriter, r *http.Request, store *persistence.Store, buf *buffer.Buffer, startedAt time.Time) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pending, oldestAge, lastFlushUnix, lastFlushErr := buf.Health()

	resp := models.HealthResponse{
		OK:                  true,
		UptimeSec:           int64(time.Since(startedAt).Seconds()),
		RollupsReady:        store.RollupsReady(),
		PendingSamples:      pending,
		OldestPendingAgeSec: oldestAge,
		LastFlushUnix:       lastFlushUnix,
		LastFlushError:      lastFlushErr,
	}

	dbSize, err := store.DatabaseSizeBytes()
	if err != nil {
		resp.OK = false
		writeJSONStatus(w, http.StatusServiceUnavailable, resp)
		return
	}
	resp.DBSizeBytes = dbSize

	lastTemp, hasLast, err := latestSample(store, buf)
	if err != nil {
		resp.OK = false
		writeJSONStatus(w, http.StatusServiceUnavailable, resp)
		return
	}
	if hasLast {
		ts := lastTemp.Timestamp
		age := time.Now().Unix() - ts
		if age < 0 {
			age = 0
		}
		resp.LastSampleTs = &ts
		resp.LastSampleAgeSec = &age
	}

	writeJSON(w, resp)
}

func latestSample(store *persistence.Store, buf *buffer.Buffer) (models.TempPoint, bool, error) {
	if last, ok := buf.Last(); ok {
		return last, true, nil
	}
	return store.GetLastTemperature()
}

func buildResponseMeta(store *persistence.Store, hasLast bool, lastTemp models.TempPoint) (models.ResponseMeta, error) {
	dbSize, err := store.DatabaseSizeBytes()
	if err != nil {
		return models.ResponseMeta{}, err
	}

	meta := models.ResponseMeta{
		DBSizeBytes: dbSize,
	}
	if hasLast {
		age := time.Now().Unix() - lastTemp.Timestamp
		if age < 0 {
			age = 0
		}
		meta.LastSampleAgeSec = &age
	}
	return meta, nil
}

func loadTemperatureData(
	store *persistence.Store,
	buf *buffer.Buffer,
	from, to int64,
	downsampleTo int,
	lastTemp models.TempPoint,
	warningThreshold, criticalThreshold float64,
) ([]models.AggregatedTempPoint, models.Statistics, int, persistence.DataSource, error) {

	pending := buf.SnapshotRange(from, to)
	source := persistence.SelectSource(from, to, int64(downsampleTo), store.RollupsReady())
	log.Printf("loadTemperatureData: source=%s from=%d to=%d downsampleTo=%d pending=%d",
		source, from, to, downsampleTo, len(pending))

	if source == persistence.SourceRaw {
		disk, err := store.GetTemperaturesFromRange(from, to)
		if err != nil {
			return nil, models.Statistics{}, 0, source, err
		}
		temperatures := merge.TempPoints(disk, pending)
		aggregated := downsampling.Aggregate(temperatures, from, to, downsampleTo)
		stats := statistics.GenerateStatistics(
			temperatures,
			lastTemp,
			warningThreshold,
			criticalThreshold,
		)
		return aggregated, stats, len(temperatures), source, nil
	}

	rollups, err := store.GetRollups(source, from, to)
	if err != nil {
		return nil, models.Statistics{}, 0, source, err
	}

	bucketSec := source.BucketSec()
	aggregated := downsampling.AggregateRollups(rollups, from, to, downsampleTo, bucketSec)
	aggregated = merge.StitchAggregated(aggregated, pending)

	stats := statistics.GenerateStatisticsFromRollups(rollups, bucketSec, lastTemp)
	sum, count := statistics.RollupSumCount(rollups)
	stats = statistics.MergeBufferIntoRollupStats(stats, sum, count, pending, lastTemp)

	processed := len(rollups) + len(pending)
	return aggregated, stats, processed, source, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	writeJSONStatus(w, http.StatusOK, v)
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encoding error: %v", err)
	}
}

func getInt64Param(query url.Values, paramName string) (int64, error) {
	paramVal := query.Get(paramName)

	if paramVal == "" {
		return 0, fmt.Errorf("Missing request parameter [%v].", paramName)
	}

	val, err := strconv.ParseInt(paramVal, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("Failed to parse parameter [%v]=[%v]: %w", paramName, paramVal, err)
	}

	return val, nil
}

func getFloatParam(query url.Values, paramName string) (float64, error) {
	paramVal := query.Get(paramName)

	if paramVal == "" {
		return 0, fmt.Errorf("Missing request parameter [%v].", paramName)
	}

	val, err := strconv.ParseFloat(paramVal, 64)
	if err != nil {
		return 0, fmt.Errorf("Failed to parse parameter [%v]=[%v]: %w", paramName, paramVal, err)
	}

	return val, nil
}
