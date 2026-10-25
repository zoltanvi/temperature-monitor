package merge

import (
	"testing"
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

func TestStitchAggregated(t *testing.T) {
	rollups := []models.AggregatedTempPoint{{Timestamp: 50, Min: 1, Max: 2, Mean: 1.5, Count: 2}}
	buf := []models.TempPoint{{Timestamp: 90, Value: 9}}
	got := StitchAggregated(rollups, buf)
	if len(got) != 2 || got[1].Mean != 9 || got[1].Count != 1 {
		t.Fatalf("got=%+v", got)
	}
}
