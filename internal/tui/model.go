// Package tui is a terminal frontend over the shared backend service.
package tui

import (
	"context"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/recording"
	"github.com/UruhaLushia/wattscope/locales"
)

type Options struct {
	Locale       string
	Window       time.Duration
	RecordingDir string
	Capture      *backend.PacketSnapshot
	SourcePath   string
}
type View struct {
	Locale                                string
	Page                                  Page
	Focus, Action                         int
	Metric                                int
	Paused                                bool
	Message                               string
	Window                                time.Duration
	Cursor                                int
	Help, Details                         bool
	Scroll                                int
	RecordingDir                          string
	Now, LastReceived                     time.Time
	Packets                               backend.PacketSnapshot
	SelectedPacket                        uint64
	PDPaused, PacketDetails               bool
	CanCheckPDHost                        bool
	PacketScroll                          int
	PacketFilter                          string
	Filtering, HideGoodCRC, HideKeepAlive bool
	RecordingActive                       bool
	SourcePath                            string
}

func (v View) messages() locales.Messages { return locales.In(v.Locale) }

type model struct {
	service        *backend.Service
	opts           Options
	view           View
	frozen         backend.Snapshot
	frozenPackets  backend.PacketSnapshot
	width, height  int
	busy           bool
	toastUntil     time.Time
	previousFilter string
}
type tickMsg time.Time
type recordingMsg struct {
	message string
	err     error
}
type pdHostCheckMsg struct {
	enabled bool
	err     error
}

func tick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(at time.Time) tea.Msg { return tickMsg(at) })
}
func (m model) Init() tea.Cmd { return tick() }

func (m *model) notice(message string) {
	m.view.Message = message
	m.toastUntil = time.Now().Add(4 * time.Second)
}
func (m *model) setPage(page Page) {
	m.view.Page = (page + pageCount) % pageCount
	m.view.Scroll = 0
	m.view.Action = 0
	m.view.PacketDetails = false
}

func (m model) currentView(includePackets bool) View {
	v := m.view
	v.SourcePath = m.opts.SourcePath
	v.RecordingDir = m.opts.RecordingDir
	switch {
	case m.service == nil:
		v.Packets = m.frozenPackets
	case includePackets && !v.PDPaused:
		v.Packets = m.service.PacketSnapshot(0, -1)
	default:
		v.Packets = m.service.PacketSnapshot(0, 0)
		if includePackets {
			v.Packets.Packets = m.frozenPackets.Packets
		}
	}
	return v
}
func (m model) actionView() View {
	v := m.currentView(false)
	if m.service != nil {
		snapshot := m.service.Snapshot(0)
		v.RecordingActive = snapshot.Recording.Active
		v.CanCheckPDHost = snapshot.CanCheckPDHost
	}
	return v
}
func (m model) measurementSnapshot(includeHistory bool) backend.Snapshot {
	if m.service == nil {
		return backend.Snapshot{State: backend.Stopped}
	}
	if !includeHistory || m.view.Paused {
		return m.service.Snapshot(0)
	}
	return m.service.SnapshotWindow(m.view.Window)
}
func (m model) View() string {
	pdPage := m.view.Page == PagePD
	showHistory := !pdPage && (m.view.Page == PageOverview || compactScreen(m.width, m.height))
	s := m.measurementSnapshot(showHistory)
	v := m.currentView(pdPage || m.opts.SourcePath != "")
	if s.Latest != nil {
		v.LastReceived = s.Latest.Timestamp
	}
	if m.view.Paused {
		s.Latest = m.frozen.Latest
		s.Statistics = m.frozen.Statistics
		if showHistory {
			s.History = m.frozen.History
		}
	}
	v.Now = time.Now()
	return Render(s, m.width, m.height, v)
}
func (m model) scrollLimit() int {
	if !m.view.Help && !m.view.Details {
		return m.contentScrollLimit()
	}
	length := len(help(m.width, m.view.messages()))
	if m.view.Details {
		length = len(details(m.measurementSnapshot(false), m.currentView(false), m.width))
	}
	return max(0, length-max(0, m.height-2))
}
func (m model) contentScrollLimit() int {
	l := layout(m.width, m.height)
	var rows []string
	switch m.view.Page {
	case PageDevice:
		rows = details(m.measurementSnapshot(false), m.currentView(false), l.mainWidth+4)
	case PageRecordings:
		rows = recordingsRows(m.measurementSnapshot(false), l.mainWidth, m.currentView(false))
	}
	return max(0, len(rows)-max(0, l.bodyHeight-3))
}
func Run(ctx context.Context, service *backend.Service, opts Options) error {
	if service == nil && opts.Capture == nil {
		return errors.New("a live session or an offline capture is required")
	}
	opts.Locale = locales.Resolve(opts.Locale)
	if opts.Window <= 0 {
		opts.Window = time.Minute
	}
	if opts.RecordingDir == "" {
		var err error
		opts.RecordingDir, err = recording.Directory("")
		if err != nil {
			return err
		}
	}
	m := model{service: service, opts: opts, view: View{Window: opts.Window, Locale: opts.Locale, Focus: focusContent, Metric: powerMetric}, width: 100, height: 30}
	if opts.Capture != nil {
		m.frozenPackets = *opts.Capture
		m.view.Page = PagePD
		m.view.PDPaused = true
	}
	_, err := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	if ctx.Err() != nil && errors.Is(err, tea.ErrProgramKilled) {
		return nil
	}
	return err
}
