package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

type Pixel struct {
	R, G, B uint8
}

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
	lastMessage   string
	currentMode   mode
	colorInput    textinput.Model
	countPrepend  int16
}

func main() {
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
	for x := int16(0); x < chunkWidth; x++ {
		canvasChunks[x] = make([]*Chunk, chunkHeight)
		for y := int16(0); y < chunkHeight; y++ {
			cx := lx + x*chunkSize
			cy := by + y*chunkSize
			fmt.Println("Fetching chunk at", cx, cy)
			canvasChunks[x][y] = getChunk(cx, cy)
		}
	}

	// Build pixel grid
	canvas := make([][]Pixel, width)
	for canvasX := int16(0); canvasX < width; canvasX++ {
		canvas[canvasX] = make([]Pixel, height)
		for canvasY := int16(0); canvasY < height; canvasY++ {
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
	ti.Placeholder = "#000000"
	ti.Focus()
	ti.CharLimit = 7
	ti.Width = 8

	m := model{
		username:    username,
		currentMode: modeCanvas,
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
	return nil
}

func (m model) View() string {
	switch m.currentMode {
	case modeCanvas:
		return m.viewCanvas()
	case modeColorPicker:
		return m.viewColorPicker()
	default:
		return ""
	}
}

func (m model) viewCanvas() string {
	var b strings.Builder
	canvas := buildCanvas(m.cursor, m.width, m.height)
	for y := int16(0); y < m.height; y++ {
		for x := int16(0); x < m.width; x++ {
			p := canvas[x][y]
			style := lipgloss.NewStyle().Background(
				lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", p.R, p.G, p.B)),
			)
			char := "  "
			if x == int16(m.width/2) && y == int16(m.height/2) {
				style = style.Foreground(lipgloss.Color("#ffffff"))
				char = "[]"
			}
			b.WriteString(style.Render(char))
		}
		b.WriteRune('\n')
	}
	if m.lastMessage != "" {
		b.WriteString("\n" + m.lastMessage)
	}
	return b.String()
}

func (m model) viewColorPicker() string {
	col := "#000000"
	if s := m.colorInput.Value(); len(s) == 7 && s[0] == '#' {
		col = s
	}
	preview := lipgloss.NewStyle().
		Background(lipgloss.Color(col)).
		Width(2).Height(1).
		Render("  ")

	box := lipgloss.JoinHorizontal(lipgloss.Top, m.colorInput.View(), " ", preview)
	dialog := lipgloss.Place(
		int(m.width*2), int(m.height+4),
		lipgloss.Center, lipgloss.Center,
		lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1).Render(
			"Pick color (#RRGGBB):\n"+box+"\nPress Enter to confirm, Esc to cancel.",
		),
	)
	return dialog
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.currentMode {
	case modeCanvas:
		return m.updateCanvas(msg)
	case modeColorPicker:
		return m.updateColorPicker(msg)
	}
	return m, nil
}

func (m model) updateCanvas(msg tea.Msg) (tea.Model, tea.Cmd) {
	count := m.countPrepend
	if count == 0 {
		count = 1
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
			num, _ := strconv.Atoi(msg.String())
			m.countPrepend = m.countPrepend*10 + int16(num)
		case "h", "left":
			m.cursor.X -= count
			m.countPrepend = 0
		case "l", "right":
			m.cursor.X += count
			m.countPrepend = 0
		case "k", "up":
			m.cursor.Y -= count
			m.countPrepend = 0
		case "j", "down":
			m.cursor.Y += count
			m.countPrepend = 0
		case " ", "enter":
			m.countPrepend = 0
			now := time.Now()
			if last, ok := lastPlaced[m.username]; ok && now.Sub(last) < pixelRateLimit {
				remaining := pixelRateLimit - now.Sub(last)
				m.lastMessage = fmt.Sprintf("Rate limit: wait %ds", int(remaining.Seconds()))
				return m, nil
			}
			m.currentMode = modeColorPicker
			m.colorInput.SetValue("#")
		}
	case tea.WindowSizeMsg:
		m.width, m.height = int16(msg.Width/2), int16(msg.Height-1)
	}
	return m, nil
}

func (m model) updateColorPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.currentMode = modeCanvas
		case "enter":
			val := m.colorInput.Value()
			if len(val) == 7 && val[0] == '#' {
				r, g, b := hexToRGB(val)
				setPixel(m.cursor, Pixel{r, g, b})
				lastPlaced[m.username] = time.Now()
				m.lastMessage = ""
				m.currentMode = modeCanvas
			} else {
				m.lastMessage = "Invalid color format."
				m.currentMode = modeCanvas
			}
		}
	case tea.WindowSizeMsg:
		m.width, m.height = int16(msg.Width/2), int16(msg.Height-1)
	}
	m.colorInput, cmd = m.colorInput.Update(msg)
	return m, cmd
}

func hexToRGB(hex string) (uint8, uint8, uint8) {
	var r, g, b uint8
	fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	return r, g, b
}
