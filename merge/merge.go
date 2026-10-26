package merge

import (
	"sort"
	"tempMonitor/models"
)

// TempPoints merges two ascending timestamp series. When timestamps collide,
// the buffer (b) value wins.
func TempPoints(disk, buf []models.TempPoint) []models.TempPoint {
	if len(disk) == 0 {
		return append([]models.TempPoint(nil), buf...)
	}
	if len(buf) == 0 {
		return append([]models.TempPoint(nil), disk...)
	}

	out := make([]models.TempPoint, 0, len(disk)+len(buf))
	i, j := 0, 0
	for i < len(disk) && j < len(buf) {
		if disk[i].Timestamp < buf[j].Timestamp {
			out = append(out, disk[i])
			i++
		} else if buf[j].Timestamp < disk[i].Timestamp {
			out = append(out, buf[j])
			j++
		} else {
			out = append(out, buf[j])
			i++
			j++
		}
	}
	out = append(out, disk[i:]...)
	out = append(out, buf[j:]...)
	return out
}

// RollupBuckets folds unflushed raw samples into rollup buckets of width
// bucketSec (min/max/sum/count), so chart aggregation sees real extrema
// instead of one-sample degenerate points.
func RollupBuckets(
	disk []models.RollupBucket,
	buffer []models.TempPoint,
	bucketSec int64,
) []models.RollupBucket {
	if len(buffer) == 0 {
		return disk
	}
	if bucketSec <= 0 {
		bucketSec = 1
	}

	byStart := make(map[int64]models.RollupBucket, len(disk)+len(buffer)/int(bucketSec)+1)

	add := func(start int64, min, max, sum float64, count int64) {
		if existing, ok := byStart[start]; ok {
			if min < existing.Min {
				existing.Min = min
			}
			if max > existing.Max {
				existing.Max = max
			}
			existing.Sum += sum
			existing.Count += count
			byStart[start] = existing
			return
		}
		byStart[start] = models.RollupBucket{
			BucketStart: start,
			Min:         min,
			Max:         max,
			Sum:         sum,
			Count:       count,
		}
	}

	for _, b := range disk {
		add(b.BucketStart, b.Min, b.Max, b.Sum, b.Count)
	}
	for _, p := range buffer {
		start := (p.Timestamp / bucketSec) * bucketSec
		add(start, p.Value, p.Value, p.Value, 1)
	}

	out := make([]models.RollupBucket, 0, len(byStart))
	for _, b := range byStart {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].BucketStart < out[j].BucketStart
	})
	return out
}
