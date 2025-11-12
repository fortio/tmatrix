package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
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

var BrightGreen = tcolor.RGBColor{R: 0, G: 255, B: 0}

func configure(fps float64, freq, speed int) *config {
	c := config{ansipixels.NewAnsiPixels(fps), matrix{streaks: make(chan streak)}, nil, freq, speed}
	c.ap.Open()
	c.ap.GetSize()
	c.ap.ClearScreen()
	c.matrix.maxX = c.ap.H
	c.matrix.maxY = c.ap.W
	c.cells = make([][]cell, c.matrix.maxX+1)
	for i := range c.cells {
		c.cells[i] = make([]cell, c.matrix.maxY+1)
	}
	return &c
}

func (c *config) resizeConfigure() {
	*c = config{ap: c.ap, matrix: matrix{streaks: make(chan streak)}, cells: nil, freq: c.freq, speed: c.speed}
	c.ap.Open()
	c.ap.GetSize()
	c.ap.ClearScreen()
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
	c := configure(*fpsFlag, *freqFlag, *speedFlag)
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
		c.resizeConfigure()
		return nil
	}
	c.ap.SyncBackgroundColor()
	c.ap.OnResize()
	c.ap.FPSTicks(func() bool {
		c.ap.GetSize()
		if c.matrix.maxX != c.ap.H || c.matrix.maxY != c.ap.W {
			c.ap.OnResize()
		}
		select {
		case streak := <-c.matrix.streaks:
			hits++
			c.cells[streak.x][streak.y].shade = BrightGreen
			c.cells[streak.x][streak.y].char = streak.char
		default:
		}
		c.shadeCells()
		num := rand.Intn(100)
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
			c.ap.WriteAt(j, i, "%s", string(cell.char))
		}
	}
}
