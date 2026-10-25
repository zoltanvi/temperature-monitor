package merge

import "tempMonitor/models"

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

// StitchAggregated appends buffer samples as single-point aggregates after
// rollup-derived chart points (unflushed tail).
func StitchAggregated(rollups []models.AggregatedTempPoint, buffer []models.TempPoint) []models.AggregatedTempPoint {
	if len(buffer) == 0 {
		return rollups
	}

	out := make([]models.AggregatedTempPoint, 0, len(rollups)+len(buffer))
	out = append(out, rollups...)
	for _, p := range buffer {
		out = append(out, models.AggregatedTempPoint{
			Timestamp: p.Timestamp,
			Min:       p.Value,
			Max:       p.Value,
			Mean:      p.Value,
			Count:     1,
		})
	}
	return out
}
