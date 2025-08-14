package main

import (
	"fmt"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

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
			m.message = fmt.Sprintf("(%d, %d)", m.cursor.X, m.cursor.Y)
		case "l", "right":
			m.cursor.X += count
			m.countPrepend = 0
			m.message = fmt.Sprintf("(%d, %d)", m.cursor.X, m.cursor.Y)
		case "k", "up":
			m.cursor.Y -= count
			m.countPrepend = 0
			m.message = fmt.Sprintf("(%d, %d)", m.cursor.X, m.cursor.Y)
		case "j", "down":
			m.cursor.Y += count
			m.countPrepend = 0
			m.message = fmt.Sprintf("(%d, %d)", m.cursor.X, m.cursor.Y)
		case " ", "enter":
			m.countPrepend = 0
			now := time.Now()
			if last, ok := lastPlaced[m.username]; ok && now.Sub(last) < pixelRateLimit {
				remaining := pixelRateLimit - now.Sub(last)
				m.message = fmt.Sprintf("Rate limit: wait %ds", int(remaining.Seconds()))
				return m, nil
			}
			m.currentMode = modeColorPicker
			m.colorInput.SetValue("")
		}
	case tea.WindowSizeMsg:
		m.width, m.height = int16(msg.Width/2), int16(msg.Height)
	case tickMsg:
		return m, tick()
	}
	return m, nil
}

func (m model) updateColorPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd, inputCmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.currentMode = modeCanvas
			cmd = tea.ClearScreen
		case "enter":
			val := m.colorInput.Value()
			if len(val) == 6 {
				r, g, b := hexToRGB(val)
				setPixel(m.cursor, Pixel(rgbToAnsi(r, g, b)))
				fmt.Println("Setting pixel at", m.cursor, "to", Pixel(rgbToAnsi(r, g, b)))
				lastPlaced[m.username] = time.Now()
				m.currentMode = modeCanvas
			} else {
				m.message = "Invalid color format."
				m.currentMode = modeCanvas
			}
		}
	case tea.WindowSizeMsg:
		m.width, m.height = int16(msg.Width/2), int16(msg.Height-1)
	case tickMsg:
		return m, tick()
	}
	m.colorInput, inputCmd = m.colorInput.Update(msg)
	if cmd != nil {
		return m, tea.Batch(cmd, inputCmd)
	}
	return m, inputCmd
}
