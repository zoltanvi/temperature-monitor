package statistics

import "tempMonitor/models"

func GenerateStatistics(
	data []models.TempPoint,
	lastTemperature models.TempPoint,
	warningThreshold float64,
	criticalThreshold float64) models.Statistics {

	var stats models.Statistics
	stats.CurrentTemp = lastTemperature.Value
	stats.StatsApproximate = false

	if len(data) == 0 {
		return stats
	}

	stats.StartTimestamp = data[0].Timestamp
	stats.EndTimestamp = data[len(data)-1].Timestamp

	sum := 0.0
	min := data[0].Value
	max := data[0].Value
	var timeInWarning int64
	var timeInCritical int64

	for i := range data {

		value := data[i].Value

		sum += value

		if value < min {
			min = value
		}

		if value > max {
			max = value
		}

		if value >= criticalThreshold {
			if i < len(data)-1 {
				timeInCritical += data[i+1].Timestamp - data[i].Timestamp
			}
		} else if value >= warningThreshold {
			if i < len(data)-1 {
				timeInWarning += data[i+1].Timestamp - data[i].Timestamp
			}
		}
	}

	stats.Mean = sum / float64(len(data))
	stats.Min = min
	stats.Max = max
	stats.TimeInWarning = float64(timeInWarning)
	stats.TimeInCritical = float64(timeInCritical)

	return stats
}

// GenerateStatisticsFromRollups builds approximate stats from rollup buckets.
// Time-in-warning/critical are always 0; StatsApproximate is true.
func GenerateStatisticsFromRollups(
	data []models.RollupBucket,
	bucketSec int64,
	lastTemperature models.TempPoint,
) models.Statistics {

	var stats models.Statistics
	stats.StatsApproximate = true
	stats.CurrentTemp = lastTemperature.Value
	stats.TimeInWarning = 0
	stats.TimeInCritical = 0

	if len(data) == 0 {
		return stats
	}

	stats.StartTimestamp = data[0].BucketStart
	last := data[len(data)-1]
	stats.EndTimestamp = last.BucketStart + bucketSec - 1

	min := data[0].Min
	max := data[0].Max
	sum := 0.0
	var count int64

	for _, b := range data {
		if b.Min < min {
			min = b.Min
		}
		if b.Max > max {
			max = b.Max
		}
		sum += b.Sum
		count += b.Count
	}

	stats.Min = min
	stats.Max = max
	if count > 0 {
		stats.Mean = sum / float64(count)
	}

	return stats
}

// RollupSumCount returns the combined sum and count across rollup buckets.
func RollupSumCount(data []models.RollupBucket) (sum float64, count int64) {
	for _, b := range data {
		sum += b.Sum
		count += b.Count
	}
	return sum, count
}

// MergeBufferIntoRollupStats folds unflushed raw samples into rollup-based stats.
// Band times stay approximate. CurrentTemp uses lastTemperature. Min/max/mean
// combine rollup totals with buffer points.
func MergeBufferIntoRollupStats(
	stats models.Statistics,
	rollupSum float64,
	rollupCount int64,
	buffer []models.TempPoint,
	lastTemperature models.TempPoint,
) models.Statistics {
	stats.CurrentTemp = lastTemperature.Value
	if len(buffer) == 0 {
		return stats
	}

	min := stats.Min
	max := stats.Max
	sum := rollupSum
	count := rollupCount
	haveExtrema := rollupCount > 0

	if stats.StartTimestamp == 0 || buffer[0].Timestamp < stats.StartTimestamp {
		stats.StartTimestamp = buffer[0].Timestamp
	}
	lastBuf := buffer[len(buffer)-1].Timestamp
	if lastBuf > stats.EndTimestamp {
		stats.EndTimestamp = lastBuf
	}

	for _, p := range buffer {
		if !haveExtrema {
			min = p.Value
			max = p.Value
			haveExtrema = true
		} else {
			if p.Value < min {
				min = p.Value
			}
			if p.Value > max {
				max = p.Value
			}
		}
		sum += p.Value
		count++
	}

	if haveExtrema {
		stats.Min = min
		stats.Max = max
	}
	if count > 0 {
		stats.Mean = sum / float64(count)
	}
	return stats
}
