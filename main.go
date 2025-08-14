package main

import (
	"fmt"
	"log"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	xssh "golang.org/x/crypto/ssh"
)

const (
	pixelRateLimit  = 1 * time.Second
	batchUpdateRate = 100 * time.Millisecond
	chunkSize       = int16(256)
)

type Location struct {
	X, Y int16
}

type Pixel uint8

type Chunk struct {
	X, Y   int16
	Pixels [chunkSize][chunkSize]Pixel
}

var (
	chunks     = []*Chunk{}
	lastPlaced = map[string]time.Time{}
)

type mode int

const (
	modeCanvas mode = iota
	modeColorPicker
)

type model struct {
	cursor        Location
	width, height int16
	username      string
	message       string
	currentMode   mode
	colorInput    textinput.Model
	countPrepend  int16
}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(time.Second*5, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func main() {
	// Restore from backup if available
	restoreBackup()
	// Start the backup worker
	go backupWorker()
	srv, err := wish.NewServer(
		wish.WithAddress(":2222"),
		wish.WithHostKeyPath(".ssh/term_key"),
		wish.WithPublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
			return true
		}),
		wish.WithMiddleware(
			bubbletea.Middleware(tuiHandler),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Fatalf("failed to create SSH server: %v", err)
	}

	log.Printf("Starting SSH r/place on :2222 ...")
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalln(err)
	}
}

func getChunk(x int16, y int16) *Chunk {
	chunkX := int16(x / chunkSize)
	chunkY := int16(y / chunkSize)
	fmt.Println("Looking for chunk at", chunkX, chunkY)
	for _, c := range chunks {
		if c.X == chunkX && c.Y == chunkY {
			return c
		}
	}
	fmt.Println("Chunk not found, creating new one at", chunkX, chunkY)
	newChunk := &Chunk{
		X:      chunkX,
		Y:      chunkY,
		Pixels: [chunkSize][chunkSize]Pixel{},
	}
	chunks = append(chunks, newChunk)
	return newChunk
}

func buildCanvas(center Location, width int16, height int16) [][]Pixel {
	fmt.Printf("Building canvas at center (%d, %d) with width %d and height %d\n", center.X, center.Y, width, height)
	lx := center.X - width/2
	by := center.Y - height/2

	// Collect all the chunks needed
	chunkWidth := int16(width/chunkSize) + 1
	chunkHeight := int16(height/chunkSize) + 1
	canvasChunks := make([][]*Chunk, chunkWidth)
	for x := range chunkWidth {
		canvasChunks[x] = make([]*Chunk, chunkHeight)
		for y := range chunkHeight {
			cx := lx + x*chunkSize
			cy := by + y*chunkSize
			fmt.Println("Fetching chunk at", cx, cy)
			canvasChunks[x][y] = getChunk(cx, cy)
		}
	}

	// Build pixel grid
	canvas := make([][]Pixel, width)
	for canvasX := range width {
		canvas[canvasX] = make([]Pixel, height)
		for canvasY := range height {
			worldX := lx + canvasX
			worldY := by + canvasY

			// Wrap into chunk-relative coords
			pixelX := (worldX%chunkSize + chunkSize) % chunkSize
			pixelY := (worldY%chunkSize + chunkSize) % chunkSize

			chunkX := (worldX / chunkSize) - (lx / chunkSize)
			chunkY := (worldY / chunkSize) - (by / chunkSize)

			canvas[canvasX][canvasY] = canvasChunks[chunkX][chunkY].Pixels[pixelX][pixelY]
		}
	}
	return canvas
}

func setPixel(l Location, p Pixel) {
	fmt.Println("Setting pixel at", l, "to", p)
	chunk := getChunk(l.X, l.Y)
	chunkX := (l.X%chunkSize + chunkSize) % chunkSize
	chunkY := (l.Y%chunkSize + chunkSize) % chunkSize
	chunk.Pixels[chunkX][chunkY] = p
}

func tuiHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	var username string
	if pk := s.PublicKey(); pk != nil {
		username = xssh.FingerprintSHA256(pk)
	} else {
		username = s.User()
	}

	ti := textinput.New()
	ti.Placeholder = "000000"
	ti.Focus()
	ti.CharLimit = 6
	ti.Width = 6
	ti.Prompt = "#"

	m := model{
		username:    username,
		currentMode: modeCanvas,
		message:     "(0, 0)",
		colorInput:  ti,
		width:       80, // Default width
		height:      24, // Default height
		cursor:      Location{X: 0, Y: 0},
	}

	progOpts := []tea.ProgramOption{tea.WithAltScreen()}

	return m, progOpts
}

var teaProgramPlaceholder *tea.Program

func (m model) Init() tea.Cmd {
	return tick()
}
