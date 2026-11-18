package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	msgs := m.view.messages()
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.view.Scroll = min(m.view.Scroll, m.scrollLimit())
	case tickMsg:
		if !m.toastUntil.IsZero() && time.Now().After(m.toastUntil) {
			m.view.Message = ""
			m.toastUntil = time.Time{}
		}
		return m, tick()
	case recordingMsg:
		m.busy = false
		if msg.err != nil {
			m.notice(msgs.RecordingFailed(msg.err.Error()))
		} else {
			m.notice(msg.message)
		}
	case pdHostCheckMsg:
		m.busy = false
		switch {
		case msg.err != nil:
			m.notice(msgs.PdCheckFailed())
		case msg.enabled:
			m.notice(msgs.PdSavedOnlineOn())
		default:
			m.notice(msgs.PdSavedOnlineOff())
		}
	case tea.MouseMsg:
		cmd := m.mouse(msg)
		return m, cmd
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.view.Filtering {
			switch msg.Type {
			case tea.KeyEnter:
				m.view.Filtering = false
			case tea.KeyEsc:
				m.view.Filtering = false
				m.view.PacketFilter = m.previousFilter
			case tea.KeyBackspace:
				r := []rune(m.view.PacketFilter)
				if len(r) > 0 {
					m.view.PacketFilter = string(r[:len(r)-1])
				}
			case tea.KeyRunes:
				if len([]rune(m.view.PacketFilter)) < 128 {
					m.view.PacketFilter += string(msg.Runes)
				}
			case tea.KeySpace:
				if len([]rune(m.view.PacketFilter)) < 128 {
					m.view.PacketFilter += " "
				}
			}
			m.view.SelectedPacket = 0
			return m, nil
		}
		if key == "q" {
			return m, tea.Quit
		}
		if key == "esc" {
			m.closeOverlays()
			return m, nil
		}
		if m.view.Help || m.view.Details {
			if key == "up" {
				m.view.Scroll = max(0, m.view.Scroll-1)
			}
			if key == "down" {
				m.view.Scroll = min(m.view.Scroll+1, m.scrollLimit())
			}
			if key == "?" {
				m.view.Help = false
			}
			return m, nil
		}
		switch key {
		case "f1", "f2", "f3", "f4":
			m.setPage(Page(key[1] - '1'))
			m.view.Focus = focusContent
			return m, nil
		}
		if key == "tab" || key == "shift+tab" {
			delta := 1
			if key == "shift+tab" {
				delta = 2
			}
			m.view.Focus = (m.view.Focus + delta) % 3
			return m, nil
		}
		if m.view.Focus == focusNavigation {
			switch key {
			case "up", "left":
				m.setPage(m.view.Page - 1)
				return m, nil
			case "down", "right":
				m.setPage(m.view.Page + 1)
				return m, nil
			case "enter":
				m.view.Focus = focusContent
				return m, nil
			}
		}
		if m.view.Focus == focusActions {
			list := actions(m.actionView())
			switch key {
			case "left", "up":
				m.view.Action = max(0, m.view.Action-1)
				return m, nil
			case "right", "down":
				m.view.Action = min(max(0, len(list)-1), m.view.Action+1)
				return m, nil
			case "enter":
				if len(list) > 0 {
					key = list[min(m.view.Action, len(list)-1)].key
				}
			}
		}
		if m.view.Page == PagePD {
			if handled, cmd := m.packetKey(key); handled {
				return m, cmd
			}
		}
		if m.view.Page == PageDevice || m.view.Page == PageRecordings {
			switch key {
			case "up":
				m.view.Scroll = max(0, m.view.Scroll-1)
				return m, nil
			case "down":
				m.view.Scroll = min(m.view.Scroll+1, m.contentScrollLimit())
				return m, nil
			case "home":
				m.view.Scroll = 0
				return m, nil
			case "end":
				m.view.Scroll = m.contentScrollLimit()
				return m, nil
			}
		}
		switch key {
		case "?":
			m.view.Help = true
			m.view.Scroll = 0
		case "d":
			m.view.Details = true
			m.view.Scroll = 0
		case "e":
			return m, m.exportPackets()
		case "p":
			return m, m.toggleCapture()
		case "r":
			return m, m.toggleRecording()
		case "c":
			if m.service != nil {
				m.service.ResetStatistics()
				m.notice(msgs.StatisticsCleared())
				if m.view.Paused {
					m.frozen = m.service.Snapshot(-1)
					m.view.Cursor = 0
				}
			}
		default:
			if m.view.Page != PageOverview {
				return m, nil
			}
			switch key {
			case "g", "]":
				m.view.Metric = (m.view.Metric + 1) % len(channels)
			case "G", "[":
				m.view.Metric = (m.view.Metric + len(channels) - 1) % len(channels)
			case "1", "2", "3", "4", "5", "6", "7":
				m.view.Metric = int(key[0] - '1')
			case " ":
				m.view.Paused = !m.view.Paused
				if m.view.Paused && m.service != nil {
					m.frozen = m.service.Snapshot(-1)
				}
				m.view.Cursor = 0
			case "left", "shift+left", "home":
				if !m.view.Paused && m.service != nil {
					m.frozen = m.service.Snapshot(-1)
					m.view.Paused = true
				}
				n := len(historyWindow(m.frozen.History, m.view.Window))
				step := 1
				if key == "shift+left" {
					step = 10
				}
				m.view.Cursor = max(0, min(n-1, m.view.Cursor+step))
				if key == "home" {
					m.view.Cursor = max(0, n-1)
				}
			case "right", "shift+right":
				step := 1
				if key == "shift+right" {
					step = 10
				}
				m.view.Cursor = max(0, m.view.Cursor-step)
			case "end":
				m.view.Paused = false
				m.view.Cursor = 0
			case "+", "=":
				m.zoomMeasurements(true)
			case "-", "_":
				m.zoomMeasurements(false)
			}
		}
	}
	return m, nil
}

func (m *model) closeOverlays() {
	m.view.Help, m.view.Details, m.view.PacketDetails = false, false, false
	m.view.Scroll, m.view.PacketScroll = 0, 0
}

func (m *model) zoomMeasurements(in bool) {
	if in {
		m.view.Window = max(5*time.Second, m.view.Window/2)
		m.view.Cursor = 0
	} else {
		m.view.Window = min(24*time.Hour, m.view.Window*2)
	}
}
