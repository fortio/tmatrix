package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand/v2"
	"runtime"
	"slices"

	"fortio.org/cli"
	"fortio.org/terminal/ansipixels"
	"fortio.org/terminal/ansipixels/tcolor"
)

type config struct {
	ap     *ansipixels.AnsiPixels
	matrix matrix
	cells  [][]cell
	freq   int
	speed  int
	fade   bool
	paused bool
	ascii  bool
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
	c.matrix.maxX = c.ap.W
	c.matrix.maxY = c.ap.H
	c.cells = make([][]cell, c.matrix.maxX+1)
	for i := range c.cells {
		c.cells[i] = make([]cell, c.matrix.maxY+1)
	}
}

func main() {
	fpsFlag := flag.Float64("fps", 60., "adjust the frames per second")
	freqFlag := flag.Int("freq", 100, "adjust the percent chance each frame that a new column is spawned in")
	speedFlag := flag.Int("speed", 1, "adjust the speed of the green streaks")
	fadeFlag := flag.Bool("fade", false, "toggle whether the letters will fade away")
	flagASCII := flag.Bool("ascii", false, "use only ascii characters")
	cli.Main()
	c := config{
		ap:    ansipixels.NewAnsiPixels(*fpsFlag),
		freq:  *freqFlag,
		speed: *speedFlag,
		fade:  *fadeFlag,
		ascii: *flagASCII,
		matrix: matrix{
			streaks: make(chan streak),
			ascii:   *flagASCII,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	var errorMessage string
	err := c.ap.Open()
	if err != nil {
		errorMessage = err.Error()
		return
	}
	c.ap.HideCursor()
	defer func() {
		c.ap.ShowCursor()
		c.ap.MoveCursor(0, 0)
		cancel()
		c.ap.Restore()
		fmt.Println(errorMessage)
	}()
	c.ap.OnResize = func() error {
		c.ap.ClearScreen()
		c.resizeConfigure()
		return nil
	}
	c.ap.SyncBackgroundColor()
	_ = c.ap.OnResize()
	if !c.fade {
		errorMessage = c.RunDirect()
		return
	}
	errorMessage = c.RunGoRoutines(ctx)
}

func (c *config) shadeCells() {
	for i, row := range c.cells[:len(c.cells)-1] {
		for j, cell := range row[:len(row)-1] {
			// Skip cells that have never been initialized (char is 0)
			if cell.char == 0 {
				continue
			}
			if cell.shade.G <= 35 {
				c.ap.WriteAt(j, i, " ")
				c.cells[i][j].char = 0 // Mark as cleared
				continue
			}
			if cell.shade.B > 0 {
				c.cells[i][j].shade.B -= 15
				c.cells[i][j].shade.R -= 15
			}
			c.cells[i][j].shade.G--
			c.ap.WriteFg(c.cells[i][j].shade.Color())
			c.ap.MoveCursor(i, j)
			c.ap.WriteRune(cell.char)
		}
	}
}

func randomNum32(maxValue int32) int32 {
	return rand.Int32N(maxValue) //nolint:gosec // good enough for random effect
}

func randomNum(maxValue int) int {
	return rand.IntN(maxValue) //nolint:gosec // good enough for random effect
}

func (c *config) drawAndIncrement(streaks *[]singleThreadStreak) {
	c.ap.ClearScreen()
	toDelete := make(map[int]bool)
	for i, s := range *streaks {
		lengthChars := len(s.chars)
		if lengthChars > 5 && randomNum(20) <= 1 {
			s.doneGrowing = true
		}
		if s.y < c.ap.H {
			c.ap.WriteFg(White.Color())
			c.ap.MoveCursor(s.x, s.y)
			c.ap.WriteRune(s.chars[lengthChars-1])
		}
		for j := 1; j < lengthChars; j++ {
			char := s.chars[lengthChars-j-1]
			clr := BrightGreen
			overflowCheck := min(max(0, lengthChars-j), 255)
			clr.G -= uint8(overflowCheck) //nolint:gosec // see line above
			if clr.G < 35 {
				if s.y-(lengthChars-j)-1 > c.ap.H {
					toDelete[i] = true
				}
				continue
			}
			if s.y-j-1 >= c.ap.H {
				continue
			}
			c.ap.MoveCursor(s.x, s.y-j)
			c.ap.WriteFg(clr.Color())
			c.ap.WriteRune(char)
		}
		s.y++
		s.newChar(c.ascii)
		if s.doneGrowing {
			s.chars = s.chars[1:]
		}

		(*streaks)[i] = s
	}
	tdKeys := make([]int, 0, len(toDelete))
	for num := range toDelete {
		tdKeys = append(tdKeys, num)
	}
	slices.SortFunc(tdKeys, func(a, b int) int { return b - a })
	for _, n := range tdKeys {
		*streaks = slices.Delete(*streaks, n, n+1)
	}
}

func (c *config) RunDirect() string {
	streaks := make([]singleThreadStreak, 0)
	err := c.ap.FPSTicks(func() bool {
		if !c.paused {
			c.drawAndIncrement(&streaks)
			num := randomNum(100)
			if num < c.freq {
				streaks = append(streaks, c.matrix.newSingleThreadedStreak())
			}
		}
		return c.handleKeys()
	})
	if err != nil {
		return fmt.Sprintf("error calling fpsticks: %s", err)
	}
	return ""
}

func (c *config) RunGoRoutines(ctx context.Context) string {
	maxProcs := runtime.GOMAXPROCS(-1)
	err := c.ap.FPSTicks(func() bool {
		if !c.paused {
			select {
			case streakTick := <-c.matrix.streaks:
				c.cells[streakTick.x][streakTick.y].shade = White
				c.cells[streakTick.x][streakTick.y].char = streakTick.char
			default:
			}
			c.shadeCells()
			num := randomNum(100)
			if num < c.freq && int(c.matrix.streaksActive.Load()) < maxProcs { // !
				c.matrix.newStreak(ctx, c.speed)
			}
		}
		return c.handleKeys()
	})
	if err != nil {
		return fmt.Sprintf("error calling fpsticks: %s", err)
	}
	return ""
}

func (c *config) handleKeys() bool {
	if len(c.ap.Data) == 0 {
		return true
	}
	switch c.ap.Data[0] {
	case ' ', 'p', 'P':
		c.paused = !c.paused
		return true
	case 'q', 'Q':
		return false
	default:
		return true
	}
}
