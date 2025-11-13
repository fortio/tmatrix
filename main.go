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
	c.matrix.maxX = c.ap.H
	c.matrix.maxY = c.ap.W
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
	// hits, newStreaks := 0, 0
	var errorMessage string
	err := c.ap.Open()
	if err != nil {
		errorMessage = ("can't open")
	}
	c.ap.HideCursor()
	defer func() {
		c.ap.ShowCursor()
		c.ap.MoveCursor(0, 0)
		cancel()
		c.ap.Restore()
		fmt.Println(errorMessage)
	}()
	c.ap.SyncBackgroundColor()
	c.ap.OnResize = func() error {
		c.ap.ClearScreen()
		c.resizeConfigure()
		return nil
	}
	_ = c.ap.OnResize()
	if !c.fade {
		errorMessage = c.RuneDirect()
		return
	}
	errorMessage = c.RunGoRoutines(ctx)
}

func (c *config) shadeCells() {
	for i, row := range c.cells[:len(c.cells)-1] {
		for j, cell := range row[:len(row)-1] {
			if cell.shade.G <= 35 {
				c.ap.WriteAt(j, i, " ")
				continue
			}
			if cell.shade.B > 0 {
				c.cells[i][j].shade.B -= 15
				c.cells[i][j].shade.R -= 15
			}
			c.cells[i][j].shade.G--
			c.ap.WriteFg(c.cells[i][j].shade.Color())
			c.ap.MoveCursor(j, i)
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
		if s.x < c.ap.H {
			c.ap.WriteFg(White.Color())
			c.ap.MoveCursor(s.y, s.x)
			c.ap.WriteRune(s.chars[lengthChars-1])
		}
		for j := lengthChars - 1; j > -1; j-- {
			char := s.chars[lengthChars-j-1]
			clr := BrightGreen
			overflowCheck := min(max(0, lengthChars-j), 255)
			clr.G -= uint8(overflowCheck) //nolint:gosec // see line above
			if clr.G < 35 {
				c.ap.MoveCursor(s.y, s.x-(lengthChars-j)-1)
				c.ap.WriteFg(clr.Color())
				c.ap.WriteRune(' ')
				if s.x-(lengthChars-j)-1 >= c.ap.H {
					toDelete[i] = true
				}
				continue
			}
			if s.x-(lengthChars-j)-1 >= c.ap.H || s.x-1-(lengthChars-j) < 0 || s.x-j-1 >= c.ap.H {
				continue
			}
			c.ap.MoveCursor(s.y, s.x-j-1)
			c.ap.WriteFg(clr.Color())
			c.ap.WriteRune(char)
		}
		s.x++
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

func (c *config) RuneDirect() string {
	streaks := make([]singleThreadStreak, 0)
	err := c.ap.FPSTicks(func() bool {
		if !c.paused {
			c.drawAndIncrement(&streaks)
		}
		if len(c.ap.Data) > 0 && c.ap.Data[0] == 'q' {
			return false
		}
		if len(c.ap.Data) > 0 && (c.ap.Data[0] == 'p' || c.ap.Data[0] == ' ') {
			c.paused = !c.paused
			return true
		}
		if c.paused {
			return true
		}
		num := randomNum(100)
		if num < c.freq {
			streaks = append(streaks, c.matrix.newSingleThreadedStreak())
		}
		return true
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
		// TODO: refactor copy pasta: c.handleKeys() or some such
		if len(c.ap.Data) > 0 && (c.ap.Data[0] == 'p' || c.ap.Data[0] == ' ') {
			c.paused = !c.paused
			return true
		}
		if len(c.ap.Data) > 0 && c.ap.Data[0] == 'q' {
			return false
		}
		return true
	})
	if err != nil {
		return fmt.Sprintf("error calling fpsticks: %s", err)
	}
	return ""
}
