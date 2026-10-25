package sensor

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Sensor struct {
	path    string
	period  time.Duration
	onValue func(float64)
	stop    chan struct{}
}

func New(period time.Duration, onValue func(float64)) *Sensor {
	return &Sensor{
		path:    "/sys/class/thermal/thermal_zone0/temp",
		period:  period,
		onValue: onValue,
		stop:    make(chan struct{}),
	}
}

func (s *Sensor) Start() {
	ticker := time.NewTicker(s.period)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				v, err := s.read()
				if err == nil && s.onValue != nil {
					s.onValue(v)
				}

			case <-s.stop:
				return
			}
		}
	}()
}

func (s *Sensor) Stop() {
	close(s.stop)
}

func (s *Sensor) read() (float64, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return 0, err
	}

	sv := strings.TrimSpace(string(b))

	v, err := strconv.Atoi(sv)
	if err != nil {
		return 0, err
	}

	return float64(v) / 1000.0, nil
}
