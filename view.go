package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

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
	start := time.Now()
	var b strings.Builder
	canvas := buildCanvas(m.cursor, m.width, m.height)
	log.Printf("Canvas rendered in %s", time.Since(start))
	lastPixel := Pixel(0)
	middleX := int16(m.width / 2)
	middleY := int16(m.height / 2)
	for y := int16(0); y < m.height; y++ {
		for x := int16(0); x < m.width; x++ {
			pixel := canvas[x][y]
			if pixel != lastPixel {
				b.WriteString("\033[48;5;")
				b.WriteString(strconv.Itoa(int(canvas[x][y])))
				b.WriteString("m")
			}
			lastPixel = pixel
			if x == middleX && y == middleY {
				b.WriteString("[]")
			} else {
				b.WriteString("  ")
			}
		}
		b.WriteRune('\n')
	}
	b.WriteString("\n" + m.message)

	return b.String()
}

func (m model) viewColorPicker() string {
	col := "000000"
	if s := m.colorInput.Value(); len(s) == 6 {
		col = s
	}
	preview := lipgloss.NewStyle().
		Background(lipgloss.Color("#" + col)).
		Width(2).Height(1).
		Render("[]")

	canvas := m.viewCanvas()

	box := lipgloss.JoinHorizontal(lipgloss.Top, m.colorInput.View(), " ", preview)
	// Place the popup centered on the canvas, but don't overwrite anything outside the popup (keep the background) using ansi codes to place the popup
	canvas += fmt.Sprintf("\033[%d;%dHColor hex: %s\033[0m", int(m.height/2)-1, int(m.width)-20, box)
	return canvas
}
