package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
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
	canvasWidth     = 40
	canvasHeight    = 20
	pixelRateLimit  = 10 * time.Second
	batchUpdateRate = 100 * time.Millisecond
)

type Pixel struct {
	R, G, B uint8
}

var (
	canvas      [canvasHeight][canvasWidth]Pixel
	canvasMutex sync.RWMutex
	lastPlaced  = map[string]time.Time{}
)

type mode int

const (
	modeCanvas mode = iota
	modeColorPicker
)

type model struct {
	cursorX, cursorY int
	username         string
	lastMessage      string
	currentMode      mode
	colorInput       textinput.Model
}

type pixelUpdateMsg struct {
	X, Y  int
	Color Pixel
}

func main() {
	for y := range canvasHeight {
		for x := range canvasWidth {
			canvas[y][x] = Pixel{0, 0, 0}
		}
	}

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
	canvasMutex.RLock()
	defer canvasMutex.RUnlock()

	var b strings.Builder
	for y := range canvasHeight {
		for x := range canvasWidth {
			p := canvas[y][x]
			style := lipgloss.NewStyle().Background(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", p.R, p.G, p.B)))
			char := "  "
			if m.cursorX == x && m.cursorY == y {
				style = style.Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", p.R, p.G, p.B)))
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
		canvasWidth*2, canvasHeight+2,
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
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "h", "left":
			if m.cursorX > 0 {
				m.cursorX--
			}
		case "l", "right":
			if m.cursorX < canvasWidth-1 {
				m.cursorX++
			}
		case "k", "up":
			if m.cursorY > 0 {
				m.cursorY--
			}
		case "j", "down":
			if m.cursorY < canvasHeight-1 {
				m.cursorY++
			}
		case " ", "enter":
			now := time.Now()
			if last, ok := lastPlaced[m.username]; ok && now.Sub(last) < pixelRateLimit {
				remaining := pixelRateLimit - now.Sub(last)
				m.lastMessage = fmt.Sprintf("Rate limit: wait %ds", int(remaining.Seconds()))
				return m, nil
			}
			m.currentMode = modeColorPicker
			m.colorInput.SetValue("#")
		}
	case pixelUpdateMsg:
		canvasMutex.Lock()
		canvas[msg.Y][msg.X] = msg.Color
		canvasMutex.Unlock()
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
				canvasMutex.Lock()
				canvas[m.cursorY][m.cursorX] = Pixel{r, g, b}
				canvasMutex.Unlock()
				lastPlaced[m.username] = time.Now()
				m.lastMessage = ""
				m.currentMode = modeCanvas
			} else {
				m.lastMessage = "Invalid color format."
				m.currentMode = modeCanvas
			}
		}
	}
	m.colorInput, cmd = m.colorInput.Update(msg)
	return m, cmd
}

func hexToRGB(hex string) (uint8, uint8, uint8) {
	var r, g, b uint8
	fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	return r, g, b
}
