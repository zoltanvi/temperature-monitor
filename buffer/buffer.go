package buffer

import (
	"sync"
	"tempMonitor/models"
	"time"
)

// Buffer holds unflushed temperature samples in memory.
type Buffer struct {
	mu sync.RWMutex

	samples []models.TempPoint

	lastFlushUnix int64
	lastFlushErr  string
}

func New() *Buffer {
	return &Buffer{}
}

func (b *Buffer) Append(ts int64, value float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.samples = append(b.samples, models.TempPoint{Timestamp: ts, Value: value})
}

// SnapshotAll returns a copy of all pending samples.
func (b *Buffer) SnapshotAll() []models.TempPoint {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return copySamples(b.samples)
}

// SnapshotRange returns pending samples with from <= ts <= to.
func (b *Buffer) SnapshotRange(from, to int64) []models.TempPoint {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]models.TempPoint, 0)
	for _, s := range b.samples {
		if s.Timestamp >= from && s.Timestamp <= to {
			out = append(out, s)
		}
	}
	return out
}

// Last returns the newest pending sample, if any.
func (b *Buffer) Last() (models.TempPoint, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if len(b.samples) == 0 {
		return models.TempPoint{}, false
	}
	return b.samples[len(b.samples)-1], true
}

// RemoveThroughTimestamp drops pending samples with ts <= maxTS after a successful flush.
func (b *Buffer) RemoveThroughTimestamp(maxTS int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	i := 0
	for i < len(b.samples) && b.samples[i].Timestamp <= maxTS {
		i++
	}
	if i == 0 {
		return
	}
	b.samples = append([]models.TempPoint(nil), b.samples[i:]...)
}

func (b *Buffer) SetFlushResult(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil {
		b.lastFlushUnix = time.Now().Unix()
		b.lastFlushErr = ""
		return
	}
	b.lastFlushErr = err.Error()
}

// Health returns pending sample count, optional oldest pending age, last flush unix, and last flush error.
func (b *Buffer) Health() (pending int, oldestAgeSec *int64, lastFlushUnix int64, lastFlushErr string) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	pending = len(b.samples)
	lastFlushUnix = b.lastFlushUnix
	lastFlushErr = b.lastFlushErr

	if pending > 0 {
		age := time.Now().Unix() - b.samples[0].Timestamp
		if age < 0 {
			age = 0
		}
		oldestAgeSec = &age
	}
	return pending, oldestAgeSec, lastFlushUnix, lastFlushErr
}

func copySamples(in []models.TempPoint) []models.TempPoint {
	if len(in) == 0 {
		return nil
	}
	out := make([]models.TempPoint, len(in))
	copy(out, in)
	return out
}
