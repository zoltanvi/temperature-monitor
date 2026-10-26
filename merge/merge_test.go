package merge

import (
	"testing"
	"tempMonitor/downsampling"
	"tempMonitor/models"
)

func TestTempPointsMerge(t *testing.T) {
	disk := []models.TempPoint{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 2}}
	buf := []models.TempPoint{{Timestamp: 2, Value: 20}, {Timestamp: 3, Value: 3}}
	got := TempPoints(disk, buf)
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	if got[1].Value != 20 {
		t.Fatalf("collision should prefer buffer: %+v", got[1])
	}
	if got[2].Timestamp != 3 {
		t.Fatalf("tail=%+v", got[2])
	}
}

func TestRollupBucketsMergesOverlappingMinute(t *testing.T) {
	disk := []models.RollupBucket{
		{BucketStart: 0, Min: 10, Max: 20, Sum: 30, Count: 2},
	}
	buf := []models.TempPoint{
		{Timestamp: 10, Value: 5},
		{Timestamp: 20, Value: 25},
		{Timestamp: 60, Value: 7},
		{Timestamp: 61, Value: 9},
	}
	got := RollupBuckets(disk, buf, 60)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2: %+v", len(got), got)
	}
	if got[0].BucketStart != 0 || got[0].Min != 5 || got[0].Max != 25 || got[0].Count != 4 || got[0].Sum != 60 {
		t.Fatalf("first bucket: %+v", got[0])
	}
	if got[1].BucketStart != 60 || got[1].Min != 7 || got[1].Max != 9 || got[1].Count != 2 || got[1].Sum != 16 {
		t.Fatalf("second bucket: %+v", got[1])
	}
}

func TestRollupBucketsEmptyBuffer(t *testing.T) {
	disk := []models.RollupBucket{{BucketStart: 0, Min: 1, Max: 2, Sum: 3, Count: 2}}
	got := RollupBuckets(disk, nil, 60)
	if len(got) != 1 || got[0].Sum != 3 {
		t.Fatalf("got=%+v", got)
	}
}

func TestRollupBucketsBufferOnly(t *testing.T) {
	buf := []models.TempPoint{
		{Timestamp: 100, Value: 1},
		{Timestamp: 101, Value: 3},
		{Timestamp: 200, Value: 2},
	}
	got := RollupBuckets(nil, buf, 60)
	if len(got) != 2 {
		t.Fatalf("len=%d: %+v", len(got), got)
	}
	// 100/60*60 = 60, 200/60*60 = 180
	if got[0].BucketStart != 60 || got[0].Min != 1 || got[0].Max != 3 || got[0].Count != 2 || got[0].Sum != 4 {
		t.Fatalf("first=%+v", got[0])
	}
	if got[1].BucketStart != 180 || got[1].Min != 2 || got[1].Max != 2 || got[1].Count != 1 || got[1].Sum != 2 {
		t.Fatalf("second=%+v", got[1])
	}
}

// Regression: large unflushed buffer must become real 1m min/max/mean chart
// points after RollupBuckets + AggregateRollups (not n=1 degenerates).
func TestRollupBucketsThenAggregateKeepsExtrema(t *testing.T) {
	disk := []models.RollupBucket{
		{BucketStart: 0, Min: 10, Max: 20, Sum: 150, Count: 10},
	}
	buf := make([]models.TempPoint, 0, 120)
	for i := int64(0); i < 120; i++ {
		buf = append(buf, models.TempPoint{Timestamp: 60 + i, Value: float64(i % 10)})
	}
	merged := RollupBuckets(disk, buf, 60)
	out := downsampling.AggregateRollups(merged, 0, 200, 1000, 60)
	if len(out) < 2 {
		t.Fatalf("expected multiple points, got %d", len(out))
	}
	// Buffer spans two minutes (60..179) → points with n>1 and min!=max
	var sawSpread bool
	for _, p := range out {
		if p.Count == 1 && p.Min == p.Max && p.Min == p.Mean {
			continue // ok for a truly single-sample bucket
		}
		if p.Count > 1 && p.Min < p.Max {
			sawSpread = true
		}
		if p.Count > 1 && p.Min == p.Max && p.Min == p.Mean {
			t.Fatalf("degenerate multi-sample point: %+v", p)
		}
	}
	if !sawSpread {
		t.Fatalf("expected at least one point with min<max from buffer: %+v", out)
	}
}
