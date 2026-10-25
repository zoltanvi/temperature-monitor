package persistence

import (
	"math"
	"path/filepath"
	"testing"
	"tempMonitor/downsampling"
	"tempMonitor/models"
	"tempMonitor/statistics"
)

func TestSelectSource(t *testing.T) {
	tests := []struct {
		name         string
		from, to     int64
		downsampleTo int64
		rollupsReady bool
		want         DataSource
	}{
		{"not ready", 0, 604800, 1500, false, SourceRaw},
		{"1h raw", 0, 3600, 1500, true, SourceRaw},
		{"1d may be raw", 0, 86400, 1500, true, SourceRaw}, // 1440 < 1500
		{"2d uses 1m", 0, 172800, 1500, true, SourceRollup1m},
		{"1w uses 1m", 0, 604800, 1500, true, SourceRollup1m},
		{"1m uses 1m", 0, 2592000, 1500, true, SourceRollup1m},
		{"long coarse enough for 1h", 0, 604800, 100, true, SourceRollup1h},
		{"month coarse enough for 1d", 0, 2592000, 20, true, SourceRollup1d},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectSource(tt.from, tt.to, tt.downsampleTo, tt.rollupsReady)
			if got != tt.want {
				t.Fatalf("SelectSource() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInsertUpdatesRollups(t *testing.T) {
	store := openTempStore(t)

	base := int64(1_700_000_000)
	if err := store.insertAt(base, 10); err != nil {
		t.Fatal(err)
	}
	if err := store.insertAt(base+1, 20); err != nil {
		t.Fatal(err)
	}
	if err := store.insertAt(base+3600, 30); err != nil {
		t.Fatal(err)
	}

	rows, err := store.GetRollups(SourceRollup1m, base, base+3600)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("1m rollups: got %d want 2", len(rows))
	}
	if rows[0].Count != 2 || rows[0].Min != 10 || rows[0].Max != 20 || rows[0].Sum != 30 {
		t.Fatalf("unexpected first 1m bucket: %+v", rows[0])
	}

	hRows, err := store.GetRollups(SourceRollup1h, base, base+3600)
	if err != nil {
		t.Fatal(err)
	}
	if len(hRows) != 2 {
		t.Fatalf("1h rollups: got %d want 2", len(hRows))
	}
}

func TestBackfillAndReady(t *testing.T) {
	store := openTempStore(t)

	base := int64(1_700_000_000)
	for i := int64(0); i < 5; i++ {
		if err := store.insertAt(base+i, float64(i+1)); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.BackfillRollups(); err != nil {
		t.Fatal(err)
	}
	if !store.RollupsReady() {
		t.Fatal("expected rollups ready after backfill")
	}

	rows, err := store.GetRollups(SourceRollup1m, base, base+4)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 minute bucket, got %d", len(rows))
	}
	if rows[0].Count != 5 || rows[0].Min != 1 || rows[0].Max != 5 || rows[0].Sum != 15 {
		t.Fatalf("unexpected backfilled bucket: %+v", rows[0])
	}
}

func TestAggregateRollupsWeightedMean(t *testing.T) {
	data := []models.RollupBucket{
		{BucketStart: 0, Min: 1, Max: 3, Sum: 10, Count: 2},
		{BucketStart: 60, Min: 0, Max: 10, Sum: 100, Count: 10},
	}
	out := downsampling.AggregateRollups(data, 0, 120, 1, 60)
	if len(out) != 1 {
		t.Fatalf("got %d points, want 1", len(out))
	}
	wantMean := 110.0 / 12.0
	if math.Abs(out[0].Mean-wantMean) > 1e-9 {
		t.Fatalf("mean = %v, want %v (not avg of means)", out[0].Mean, wantMean)
	}
	if out[0].Min != 0 || out[0].Max != 10 || out[0].Count != 12 {
		t.Fatalf("unexpected agg: %+v", out[0])
	}
}

func TestStatsFromRollupsApproximate(t *testing.T) {
	data := []models.RollupBucket{
		{BucketStart: 100, Min: 1, Max: 5, Sum: 9, Count: 3},
		{BucketStart: 160, Min: 2, Max: 8, Sum: 20, Count: 4},
	}
	stats := statistics.GenerateStatisticsFromRollups(data, 60, models.TempPoint{Value: 7})
	if !stats.StatsApproximate {
		t.Fatal("expected approximate")
	}
	if stats.TimeInWarning != 0 || stats.TimeInCritical != 0 {
		t.Fatal("band times must be 0")
	}
	if stats.Min != 1 || stats.Max != 8 {
		t.Fatalf("min/max: got %v/%v", stats.Min, stats.Max)
	}
	wantMean := 29.0 / 7.0
	if math.Abs(stats.Mean-wantMean) > 1e-9 {
		t.Fatalf("mean = %v want %v", stats.Mean, wantMean)
	}
	if stats.CurrentTemp != 7 {
		t.Fatalf("current = %v", stats.CurrentTemp)
	}
	if stats.EndTimestamp != 160+60-1 {
		t.Fatalf("end = %v", stats.EndTimestamp)
	}
}

func TestTransactionalInsertNoOrphan(t *testing.T) {
	store := openTempStore(t)
	_ = store.Close()

	err := store.insertAt(1_700_000_000, 42)
	if err == nil {
		t.Fatal("expected error on closed db")
	}
}

func openTempStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := OpenDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
