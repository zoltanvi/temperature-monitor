package persistence

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"tempMonitor/models"
	"time"

	_ "modernc.org/sqlite"
)

const (
	BucketSec1m = 60
	BucketSec1h = 3600
	BucketSec1d = 86400

	metaKeyRollupsReady             = "rollups_ready"
	metaKeyBackfillStartedAt        = "rollups_backfill_started_at"
	metaKeyBackfillFinishedAt       = "rollups_backfill_finished_at"
	backfillChunkSize               = 10000
)

type DataSource int

const (
	SourceRaw DataSource = iota
	SourceRollup1m
	SourceRollup1h
	SourceRollup1d
)

func (s DataSource) BucketSec() int64 {
	switch s {
	case SourceRollup1m:
		return BucketSec1m
	case SourceRollup1h:
		return BucketSec1h
	case SourceRollup1d:
		return BucketSec1d
	default:
		return 0
	}
}

func (s DataSource) String() string {
	switch s {
	case SourceRaw:
		return "raw"
	case SourceRollup1m:
		return "1m"
	case SourceRollup1h:
		return "1h"
	case SourceRollup1d:
		return "1d"
	default:
		return "unknown"
	}
}

type Store struct {
	db   *sql.DB
	path string
}

func OpenDatabase(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	s := &Store{db: db, path: path}

	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) init() error {
	_, err := s.db.Exec(`
		PRAGMA journal_mode=WAL;
		PRAGMA synchronous=NORMAL;
		PRAGMA busy_timeout = 3000;

		CREATE TABLE IF NOT EXISTS temperature_samples (
			ts INTEGER NOT NULL PRIMARY KEY,
			value REAL NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_temperature_samples_ts
		ON temperature_samples(ts);

		CREATE TABLE IF NOT EXISTS temperature_rollups_1m (
			bucket_start INTEGER NOT NULL PRIMARY KEY,
			min REAL NOT NULL,
			max REAL NOT NULL,
			sum REAL NOT NULL,
			count INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS temperature_rollups_1h (
			bucket_start INTEGER NOT NULL PRIMARY KEY,
			min REAL NOT NULL,
			max REAL NOT NULL,
			sum REAL NOT NULL,
			count INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS temperature_rollups_1d (
			bucket_start INTEGER NOT NULL PRIMARY KEY,
			min REAL NOT NULL,
			max REAL NOT NULL,
			sum REAL NOT NULL,
			count INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS temperature_meta (
			key TEXT NOT NULL PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	return err
}

func (s *Store) Insert(value float64) error {
	return s.insertAt(time.Now().Unix(), value)
}

func (s *Store) insertAt(ts int64, value float64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO temperature_samples(ts, value) VALUES (?, ?)`,
		ts,
		value,
	); err != nil {
		return err
	}

	if err := upsertRollupTx(tx, "temperature_rollups_1m", BucketSec1m, ts, value); err != nil {
		return err
	}
	if err := upsertRollupTx(tx, "temperature_rollups_1h", BucketSec1h, ts, value); err != nil {
		return err
	}
	if err := upsertRollupTx(tx, "temperature_rollups_1d", BucketSec1d, ts, value); err != nil {
		return err
	}

	return tx.Commit()
}

func upsertRollupTx(tx *sql.Tx, table string, bucketSec, ts int64, value float64) error {
	bucketStart := (ts / bucketSec) * bucketSec
	_, err := tx.Exec(fmt.Sprintf(`
		INSERT INTO %s(bucket_start, min, max, sum, count)
		VALUES (?, ?, ?, ?, 1)
		ON CONFLICT(bucket_start) DO UPDATE SET
			min = MIN(min, excluded.min),
			max = MAX(max, excluded.max),
			sum = sum + excluded.sum,
			count = count + excluded.count
	`, table), bucketStart, value, value, value)
	return err
}

// Returns a slice of samples [FROM - TO] where FROM and TO is inclusive
func (s *Store) GetTemperaturesFromRange(from, to int64) ([]models.TempPoint, error) {

	log.Printf("GetSlice: from=%d to=%d", from, to)

	rows, err := s.db.Query(`
		SELECT ts, value
		FROM temperature_samples
		WHERE ts >= ? and ts <= ?
		ORDER BY ts ASC
	`, from, to)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	result := make([]models.TempPoint, 0)

	for rows.Next() {
		var timestamp int64
		var tempVal float64

		if err := rows.Scan(&timestamp, &tempVal); err != nil {
			return nil, err
		}

		result = append(result, models.TempPoint{
			Timestamp: timestamp,
			Value:     tempVal,
		})
	}

	return result, rows.Err()
}

// GetRollups returns rollup buckets overlapping [from, to] for the given source.
func (s *Store) GetRollups(source DataSource, from, to int64) ([]models.RollupBucket, error) {
	table, bucketSec, err := rollupTable(source)
	if err != nil {
		return nil, err
	}

	startBound := (from / bucketSec) * bucketSec

	log.Printf("GetRollups(%s): from=%d to=%d startBound=%d", source, from, to, startBound)

	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT bucket_start, min, max, sum, count
		FROM %s
		WHERE bucket_start >= ? AND bucket_start <= ?
		ORDER BY bucket_start ASC
	`, table), startBound, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]models.RollupBucket, 0)
	for rows.Next() {
		var b models.RollupBucket
		if err := rows.Scan(&b.BucketStart, &b.Min, &b.Max, &b.Sum, &b.Count); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

func rollupTable(source DataSource) (table string, bucketSec int64, err error) {
	switch source {
	case SourceRollup1m:
		return "temperature_rollups_1m", BucketSec1m, nil
	case SourceRollup1h:
		return "temperature_rollups_1h", BucketSec1h, nil
	case SourceRollup1d:
		return "temperature_rollups_1d", BucketSec1d, nil
	default:
		return "", 0, fmt.Errorf("not a rollup source: %v", source)
	}
}

// GetLastTemperature returns the latest sample.
// ok is false when the table is empty (not an error).
func (s *Store) GetLastTemperature() (models.TempPoint, bool, error) {
	var tp models.TempPoint

	err := s.db.QueryRow(`
		SELECT ts, value
		FROM temperature_samples
		ORDER BY ts DESC
		LIMIT 1
	`).Scan(&tp.Timestamp, &tp.Value)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.TempPoint{}, false, nil
		}
		return models.TempPoint{}, false, fmt.Errorf("Failed to query last temperature: %w", err)
	}

	return tp, true, nil
}

// DatabaseSizeBytes returns the on-disk size of the main DB file.
func (s *Store) DatabaseSizeBytes() (int64, error) {
	info, err := os.Stat(s.path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// Path returns the database file path.
func (s *Store) Path() string {
	return s.path
}

func (s *Store) RollupsReady() bool {
	v, err := s.getMeta(metaKeyRollupsReady)
	if err != nil {
		return false
	}
	return v == "1"
}

func (s *Store) setRollupsReady(ready bool) error {
	val := "0"
	if ready {
		val = "1"
	}
	return s.setMeta(metaKeyRollupsReady, val)
}

func (s *Store) getMeta(key string) (string, error) {
	var value string
	err := s.db.QueryRow(
		`SELECT value FROM temperature_meta WHERE key = ?`,
		key,
	).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

func (s *Store) setMeta(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO temperature_meta(key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

// SelectSource picks the coarsest rollup whose expected bucket count is still
// >= downsampleTo. Falls back to raw when rollups are not ready or no rollup
// is fine enough.
func SelectSource(from, to, downsampleTo int64, rollupsReady bool) DataSource {
	if !rollupsReady || downsampleTo <= 0 {
		return SourceRaw
	}

	rangeSec := to - from
	if rangeSec <= 0 {
		return SourceRaw
	}

	candidates := []DataSource{SourceRollup1d, SourceRollup1h, SourceRollup1m}
	for _, src := range candidates {
		bucketSec := src.BucketSec()
		expected := (rangeSec + bucketSec - 1) / bucketSec // ceil
		if expected >= downsampleTo {
			return src
		}
	}
	return SourceRaw
}

// BackfillRollups rebuilds all rollup tables from raw samples and sets rollups_ready.
// Idempotent: safe to re-run. Prefer calling before the sensor ticker starts.
func (s *Store) BackfillRollups() error {
	log.Println("BackfillRollups: starting")
	_ = s.setRollupsReady(false)
	_ = s.setMeta(metaKeyBackfillStartedAt, fmt.Sprintf("%d", time.Now().Unix()))

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, table := range []string{
		"temperature_rollups_1m",
		"temperature_rollups_1h",
		"temperature_rollups_1d",
	} {
		if _, err := tx.Exec(`DELETE FROM ` + table); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	rows, err := s.db.Query(`
		SELECT ts, value
		FROM temperature_samples
		ORDER BY ts ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var (
		txChunk   *sql.Tx
		inChunk   int
		processed int64
	)

	commitChunk := func() error {
		if txChunk == nil {
			return nil
		}
		if err := txChunk.Commit(); err != nil {
			return err
		}
		txChunk = nil
		inChunk = 0
		return nil
	}

	for rows.Next() {
		var ts int64
		var value float64
		if err := rows.Scan(&ts, &value); err != nil {
			_ = commitChunk()
			return err
		}

		if txChunk == nil {
			txChunk, err = s.db.Begin()
			if err != nil {
				return err
			}
		}

		if err := upsertRollupTx(txChunk, "temperature_rollups_1m", BucketSec1m, ts, value); err != nil {
			txChunk.Rollback()
			return err
		}
		if err := upsertRollupTx(txChunk, "temperature_rollups_1h", BucketSec1h, ts, value); err != nil {
			txChunk.Rollback()
			return err
		}
		if err := upsertRollupTx(txChunk, "temperature_rollups_1d", BucketSec1d, ts, value); err != nil {
			txChunk.Rollback()
			return err
		}

		inChunk++
		processed++
		if inChunk >= backfillChunkSize {
			if err := commitChunk(); err != nil {
				return err
			}
			if processed%100000 == 0 {
				log.Printf("BackfillRollups: processed %d samples", processed)
			}
		}
	}

	if err := rows.Err(); err != nil {
		if txChunk != nil {
			txChunk.Rollback()
		}
		return err
	}

	if err := commitChunk(); err != nil {
		return err
	}

	if err := s.setMeta(metaKeyBackfillFinishedAt, fmt.Sprintf("%d", time.Now().Unix())); err != nil {
		return err
	}
	if err := s.setRollupsReady(true); err != nil {
		return err
	}

	log.Printf("BackfillRollups: complete (%d samples)", processed)
	return nil
}

// EnsureRollupsReady runs backfill if rollups are not marked ready.
func (s *Store) EnsureRollupsReady() error {
	if s.RollupsReady() {
		log.Println("Rollups already ready; skipping backfill")
		return nil
	}
	return s.BackfillRollups()
}
