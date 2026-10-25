package statistics

import (
	"math"
	"testing"
	"tempMonitor/models"
)

func TestMergeBufferIntoRollupStats(t *testing.T) {
	stats := models.Statistics{
		StatsApproximate: true,
		StartTimestamp:   100,
		EndTimestamp:     159,
		Min:              1,
		Max:              5,
		Mean:             3,
		CurrentTemp:      5,
	}
	buf := []models.TempPoint{
		{Timestamp: 160, Value: 0},
		{Timestamp: 161, Value: 10},
	}
	last := models.TempPoint{Timestamp: 161, Value: 10}

	got := MergeBufferIntoRollupStats(stats, 9, 3, buf, last)
	if got.CurrentTemp != 10 {
		t.Fatalf("current=%v", got.CurrentTemp)
	}
	if got.Min != 0 || got.Max != 10 {
		t.Fatalf("min/max=%v/%v", got.Min, got.Max)
	}
	wantMean := (9.0 + 0 + 10) / 5.0
	if math.Abs(got.Mean-wantMean) > 1e-9 {
		t.Fatalf("mean=%v want %v", got.Mean, wantMean)
	}
	if got.EndTimestamp != 161 {
		t.Fatalf("end=%v", got.EndTimestamp)
	}
	if !got.StatsApproximate {
		t.Fatal("should stay approximate")
	}
}
