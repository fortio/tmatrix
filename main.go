package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand/v2"
	"runtime"

	"fortio.org/terminal/ansipixels"
	"fortio.org/terminal/ansipixels/tcolor"
)

type config struct {
	ap     *ansipixels.AnsiPixels
	matrix matrix
	cells  [][]cell
	freq   int
	speed  int
}

type cell struct {
	char  rune
	shade tcolor.RGBColor
}

var (
	BrightGreen = tcolor.RGBColor{R: 0, G: 255, B: 0}
	White       = tcolor.RGBColor{R: 255, G: 255, B: 255}
)

func (c *config) resizeConfigure() {
	*c = config{ap: c.ap, matrix: matrix{streaks: make(chan streak)}, cells: nil, freq: c.freq, speed: c.speed}
	c.matrix.maxX = c.ap.H
	c.matrix.maxY = c.ap.W
	c.cells = make([][]cell, c.matrix.maxX+1)
	for i := range c.cells {
		c.cells[i] = make([]cell, c.matrix.maxY+1)
	}
}

func main() {
	maxProcs := int32(runtime.GOMAXPROCS(-1))
	fpsFlag := flag.Float64("fps", 60., "adjust the frames per second")
	freqFlag := flag.Int("freq", 2, "adjust the percent chance each frame that a new column is spawned in")
	speedFlag := flag.Int("speed", 1, "adjust the speed of the green streaks")
	flag.Parse()
	c := config{ap: ansipixels.NewAnsiPixels(*fpsFlag), freq: *freqFlag, speed: *speedFlag}
	ctx, cancel := context.WithCancel(context.Background())
	hits, newStreaks := 0, 0
	c.ap.HideCursor()
	defer func() {
		c.ap.ClearScreen()
		c.ap.ShowCursor()
		c.ap.MoveCursor(0, 0)
		c.ap.Restore()
		cancel()
	}()
	c.ap.OnResize = func() error {
		c.ap.Open()
		c.ap.GetSize()
		c.ap.ClearScreen()
		c.resizeConfigure()
		return nil
	}
	_ = c.ap.OnResize()
	c.ap.SyncBackgroundColor()
	c.ap.FPSTicks(func() bool {
		select {
		case streak := <-c.matrix.streaks:
			hits++
			c.cells[streak.x][streak.y].shade = BrightGreen
			c.cells[streak.x][streak.y].char = streak.char
		default:
		}
		c.shadeCells()
		num := int(rand.Int32N(100))
		if num <= c.freq && c.matrix.streaksActive.Load() < maxProcs {
			c.matrix.newStreak(ctx, c.speed)
			newStreaks++
		}
		if len(c.ap.Data) > 0 && c.ap.Data[0] == 'q' {
			return false
		}
		return true
	})
	fmt.Println(len(c.matrix.streaks))
}

func (c *config) shadeCells() {
	for i, row := range c.cells[:len(c.cells)-1] {
		for j, cell := range row[:len(row)-1] {
			if cell.shade.G <= 35 {
				c.ap.WriteAt(j, i, " ")
				continue
			}
			c.cells[i][j].shade.G--
			c.ap.WriteFg(c.cells[i][j].shade.Color())
			c.ap.MoveCursor(j, i)
			c.ap.WriteRune(cell.char)
		}
	}
}
