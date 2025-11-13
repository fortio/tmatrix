package main

import (
	"context"
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
	ascii         bool
}

func getRandomRune(ascii bool) rune {
	if ascii {
		// number between 33 and 126 = nice ascii char
		return randomNum32(127-33) + int32(33)
	}
	// katakana
	start := int32(0x30A0)
	end := int32(0x30FF)
	return start + randomNum32(end-start)
}

func (m *matrix) newStreak(ctx context.Context, speedDividend int) {
	s := streak{
		randomNum(m.maxX), randomNum(m.maxY),
		getRandomRune(m.ascii),
	}
	speed := randomNum(100)
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
				s.char = getRandomRune(m.ascii)
				m.streaks <- s
			case <-ctx.Done():
				return
			}
		}
	}()
}

type singleThreadStreak struct {
	chars       []rune
	x, y        int
	doneGrowing bool
}

func (sts *singleThreadStreak) newChar(ascii bool) {
	sts.chars = append(sts.chars, getRandomRune(ascii))
}

func (m *matrix) newSingleThreadedStreak() singleThreadStreak {
	s := singleThreadStreak{
		[]rune{getRandomRune(m.ascii)},
		0, randomNum(m.maxY),
		// randomNum(int32(m.maxX)), randomNum(int32(m.maxY)), //nolint:gosec // Only would overflow if terminal size is massive
		false,
	}
	return s
}
