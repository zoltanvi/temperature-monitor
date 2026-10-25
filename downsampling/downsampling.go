package downsampling

import (
	"math"
	"tempMonitor/models"

	"github.com/dgryski/go-lttb"
)

type DataPoint struct {
	Timestamp int64   `json:"ts"`
	Value     float64 `json:"v"`
}

func DownsampleLTTB(
	data []models.TempPoint,
	downsampleTo int,
) []models.TempPoint {

	if downsampleTo < 3 {
		downsampleTo = 3
	}

	if len(data) <= downsampleTo {
		return data
	}

	points := make([]lttb.Point[float64], len(data))
	for i, d := range data {
		points[i] = lttb.Point[float64]{
			X: float64(d.Timestamp),
			Y: d.Value,
		}
	}

	downsampled := lttb.LTTB(points, downsampleTo)

	result := make([]models.TempPoint, len(downsampled))
	for i, p := range downsampled {
		result[i] = models.TempPoint{
			Timestamp: int64(p.X),
			Value:     p.Y,
		}
	}

	return result
}

func Aggregate(
	data []models.TempPoint,
	from, to int64,
	downsampleTo int,
) []models.AggregatedTempPoint {

	if downsampleTo <= 0 || len(data) == 0 {
		return []models.AggregatedTempPoint{}
	}

	if len(data) <= downsampleTo {
		out := make([]models.AggregatedTempPoint, len(data))

		for i, d := range data {
			out[i] = models.AggregatedTempPoint{
				Timestamp: d.Timestamp,
				Min:       d.Value,
				Max:       d.Value,
				Mean:      d.Value,
				Count:     1,
			}
		}
		return out
	}

	bucketSec := calcBucketSec(from, to, downsampleTo)

	out := make([]models.AggregatedTempPoint, 0, downsampleTo)
	i := 0

	for i < len(data) {
		start := bucketStart(data[i].Timestamp, bucketSec)

		min := data[i].Value
		max := data[i].Value
		sum := data[i].Value
		count := 1
		i++

		for i < len(data) {
			if bucketStart(data[i].Timestamp, bucketSec) != start {
				break
			}

			v := data[i].Value
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
			sum += v
			count++
			i++
		}

		out = append(out, models.AggregatedTempPoint{
			Timestamp: start + bucketSec/2,
			Min:       min,
			Max:       max,
			Mean:      sum / float64(count),
			Count:     float64(count),
		})
	}

	return out
}

// AggregateRollups re-buckets pre-aggregated rollup rows into ~downsampleTo
// output points, merging with weighted sum/count (not average of means).
// sourceBucketSec is the stored resolution width (used for midpoints when not re-bucketing).
func AggregateRollups(
	data []models.RollupBucket,
	from, to int64,
	downsampleTo int,
	sourceBucketSec int64,
) []models.AggregatedTempPoint {

	if downsampleTo <= 0 || len(data) == 0 {
		return []models.AggregatedTempPoint{}
	}

	if len(data) <= downsampleTo {
		out := make([]models.AggregatedTempPoint, len(data))
		for i, d := range data {
			mean := 0.0
			if d.Count > 0 {
				mean = d.Sum / float64(d.Count)
			}
			ts := d.BucketStart
			if sourceBucketSec > 0 {
				ts = d.BucketStart + sourceBucketSec/2
			}
			out[i] = models.AggregatedTempPoint{
				Timestamp: ts,
				Min:       d.Min,
				Max:       d.Max,
				Mean:      mean,
				Count:     float64(d.Count),
			}
		}
		return out
	}

	bucketSec := calcBucketSec(from, to, downsampleTo)
	out := make([]models.AggregatedTempPoint, 0, downsampleTo)
	i := 0

	for i < len(data) {
		start := bucketStart(data[i].BucketStart, bucketSec)

		min := data[i].Min
		max := data[i].Max
		sum := data[i].Sum
		count := data[i].Count
		i++

		for i < len(data) {
			if bucketStart(data[i].BucketStart, bucketSec) != start {
				break
			}
			if data[i].Min < min {
				min = data[i].Min
			}
			if data[i].Max > max {
				max = data[i].Max
			}
			sum += data[i].Sum
			count += data[i].Count
			i++
		}

		mean := 0.0
		if count > 0 {
			mean = sum / float64(count)
		}
		out = append(out, models.AggregatedTempPoint{
			Timestamp: start + bucketSec/2,
			Min:       min,
			Max:       max,
			Mean:      mean,
			Count:     float64(count),
		})
	}

	return out
}

// calcBucketSec derives a fixed-width bucket size from the query window and
// target point count, snapped to Grafana-style "nice" intervals.
func calcBucketSec(from, to int64, downsampleTo int) int64 {
	rangeSec := to - from
	if rangeSec <= 0 || downsampleTo <= 0 {
		return 1
	}

	target := int64(math.Ceil(float64(rangeSec) / float64(downsampleTo)))
	if target <= 0 {
		return 1
	}

	return roundInterval(target)
}

// roundInterval snaps a duration in seconds up to a standard time-series step.
func roundInterval(seconds int64) int64 {
	switch {
	case seconds <= 1:
		return 1
	case seconds <= 2:
		return 2
	case seconds <= 5:
		return 5
	case seconds <= 10:
		return 10
	case seconds <= 15:
		return 15
	case seconds <= 20:
		return 20
	case seconds <= 30:
		return 30
	case seconds <= 60:
		return 60
	case seconds <= 120:
		return 120
	case seconds <= 180:
		return 180
	case seconds <= 300:
		return 300
	case seconds <= 600:
		return 600
	case seconds <= 900:
		return 900
	case seconds <= 1200:
		return 1200
	case seconds <= 1800:
		return 1800
	case seconds <= 3600:
		return 3600
	case seconds <= 7200:
		return 7200
	case seconds <= 10800:
		return 10800
	case seconds <= 21600:
		return 21600
	case seconds <= 43200:
		return 43200
	case seconds <= 86400:
		return 86400
	default:
		days := (seconds + 86399) / 86400
		switch {
		case days <= 1:
			return 86400
		case days <= 2:
			return 2 * 86400
		case days <= 5:
			return 5 * 86400
		case days <= 7:
			return 7 * 86400
		case days <= 14:
			return 14 * 86400
		case days <= 30:
			return 30 * 86400
		default:
			return ((days + 29) / 30) * 30 * 86400
		}
	}
}

func bucketStart(ts, bucketSec int64) int64 {
	return (ts / bucketSec) * bucketSec
}
