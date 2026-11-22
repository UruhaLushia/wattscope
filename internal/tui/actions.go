package tui

import (
	"context"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/UruhaLushia/wattscope/internal/recording"
)

type action struct {
	key, label string
	active     bool
}

func (a action) text() string {
	key := a.key
	if key == " " {
		key = "Space"
	}
	return key + " " + a.label
}

func actions(v View) []action {
	m := v.messages()
	record := m.ActionRecord()
	if v.RecordingActive {
		record = m.ActionStop()
	}
	capture := m.PdRecordStart()
	if v.Packets.Recording.Active {
		capture = m.PdRecordStop()
	}
	hold := m.ActionHold()
	paused := v.Page == PagePD && v.PDPaused || v.Page == PageOverview && v.Paused
	if paused {
		hold = m.ActionResume()
	}
	var list []action
	switch v.Page {
	case PagePD:
		if v.SourcePath == "" {
			list = append(list, action{key: "r", label: capture, active: v.Packets.Recording.Active})
			if v.CanCheckPDHost {
				list = append(list, action{key: "p", label: m.PdCheckOnline()})
			}
		}
		list = append(list,
			action{key: "e", label: m.ExportPackets()},
			action{key: "/", label: m.ActionFilter(), active: v.Filtering || v.PacketFilter != ""},
			action{key: " ", label: hold, active: paused && v.SourcePath == ""},
			action{key: "c", label: m.ActionClear()},
		)
	case PageRecordings:
		if v.SourcePath == "" {
			list = append(list,
				action{key: "r", label: record, active: v.RecordingActive},
				action{key: "p", label: capture, active: v.Packets.Recording.Active},
			)
		}
		list = append(list, action{key: "e", label: m.ExportPackets()}, action{key: "?", label: m.ActionHelp()})
	case PageDevice:
		list = append(list, action{key: "?", label: m.ActionHelp()})
	default:
		if v.SourcePath == "" {
			list = append(list, action{key: "r", label: record, active: v.RecordingActive})
		}
		list = append(list,
			action{key: "c", label: m.ActionClear()},
			action{key: "d", label: m.ActionDetails()},
			action{key: " ", label: hold, active: paused},
			action{key: "?", label: m.ActionHelp()},
		)
	}
	return append(list, action{key: "q", label: m.ActionQuit()})
}

func (m *model) checkPDHost() tea.Cmd {
	if m.busy || m.service == nil || !m.service.Snapshot(0).CanCheckPDHost {
		return nil
	}
	m.busy = true
	m.notice(m.view.messages().PdCheckingOnline())
	service := m.service
	return func() tea.Msg {
		enabled, err := service.CheckSavedPDHost(context.Background())
		return pdHostCheckMsg{enabled: enabled, err: err}
	}
}

func actionBar(width int, v View) string {
	var labels []string
	used := 0
	list := actions(v)
	start, end := actionWindow(list, width, v)
	for i := start; i < end; i++ {
		item := list[i]
		style := buttonStyle
		if item.active {
			style = activeButtonStyle
		}
		if v.Focus == focusActions && i == v.Action {
			style = focusStyle
		}
		rendered := style.Render(item.text())
		n := ansi.StringWidth(rendered)
		if used+n > width {
			break
		}
		labels = append(labels, rendered)
		used += n + 1
	}
	line := strings.Join(labels, " ")
	if width-used > ansi.StringWidth(v.messages().FocusHint())+2 {
		line = apart(line, quietStyle.Render(v.messages().FocusHint()), width)
	}
	return fit(line, width)
}

func actionWindow(list []action, width int, v View) (int, int) {
	buttonWidth := func(a action) int {
		return ansi.StringWidth(buttonStyle.Render(a.text()))
	}
	start := 0
	if v.Focus == focusActions && len(list) > 0 {
		start = max(0, min(v.Action, len(list)-1))
		used := buttonWidth(list[start])
		for start > 0 && used+buttonWidth(list[start-1])+1 <= width {
			start--
			used += buttonWidth(list[start]) + 1
		}
	}
	end, used := start, 0
	for end < len(list) {
		n := buttonWidth(list[end])
		if used+n > width {
			break
		}
		used += n + 1
		end++
	}
	return start, end
}

func (m *model) toggleRecording() tea.Cmd {
	if m.service == nil {
		m.notice(m.view.messages().CaptureNotAvailable())
		return nil
	}
	if m.busy {
		return nil
	}
	m.busy = true
	m.notice(m.view.messages().UpdatingRecording())
	service, dir, msgs := m.service, m.opts.RecordingDir, m.view.messages()
	return func() tea.Msg {
		if service.Snapshot(0).Recording.Active {
			return recordingMsg{msgs.RecordingSaved(), service.StopRecording()}
		}
		path := recording.NextPath(dir)
		return recordingMsg{msgs.RecordingTo(path), service.StartRecording(path)}
	}
}
func (m *model) toggleCapture() tea.Cmd {
	if m.service == nil {
		m.notice(m.view.messages().CaptureNotAvailable())
		return nil
	}
	if m.busy {
		return nil
	}
	m.busy = true
	service, dir, msgs := m.service, m.opts.RecordingDir, m.view.messages()
	return func() tea.Msg {
		if service.PacketSnapshot(0, 0).Recording.Active {
			return recordingMsg{msgs.RecordingSaved(), service.StopCapture()}
		}
		path := recording.NextCapturePath(dir)
		return recordingMsg{msgs.CaptureStarted(path), service.StartCapture(path)}
	}
}
func (m *model) exportPackets() tea.Cmd {
	if m.busy {
		return nil
	}
	packets := filteredPackets(m.currentView(true))
	if len(packets) == 0 {
		m.notice(m.view.messages().EmptyCapture())
		return nil
	}
	m.busy = true
	path, msgs := recording.NextCapturePath(m.opts.RecordingDir), m.view.messages()
	return func() tea.Msg {
		return recordingMsg{msgs.ExportedPackets(strconv.Itoa(len(packets)), path), recording.ExportCapture(path, packets)}
	}
}
