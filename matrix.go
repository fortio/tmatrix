package main

import (
	"context"
	"math/rand"
	"sync/atomic"
	"time"
)

type streak struct {
	x, y int
	char rune
}

type matrix struct {
	maxX, maxY    int
	streaks       chan (streak)
	streaksActive atomic.Int32
}

func (m *matrix) newStreak(ctx context.Context, speedDividend int) {
	s := streak{rand.Intn(m.maxX), rand.Intn(m.maxY), rune(rand.Intn(128) + 8)} //nolint:gosec // good enough for random effect
	speed := rand.Intn(100)                                                     //nolint:gosec // good enough for random effect
	timeBetween := max(time.Duration(speed*int(time.Millisecond)/speedDividend), 10)
	m.streaksActive.Add(1)
	go func() {
		defer m.streaksActive.Add(-1)
		ticker := time.NewTicker(timeBetween)
		defer func() {
			ticker.Stop()
		}()
		m.streaks <- s
		for {
			select {
			case <-ticker.C:
				s.x++
				if s.x >= m.maxX {
					return
				}
				// number between 33 and 126 = nice ascii char
				s.char = rune(rand.Intn(127-33) + 33) //nolint:gosec // good enough for random effect
				m.streaks <- s
			case <-ctx.Done():
				return
			}
		}
	}()
}
