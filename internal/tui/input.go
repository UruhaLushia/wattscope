package tui

import (
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/UruhaLushia/wattscope/internal/backend"
)

func (m *model) holdPackets(snapshot backend.PacketSnapshot) {
	if !m.view.PDPaused {
		m.frozenPackets = snapshot
		m.view.PDPaused = true
	}
}

func (m *model) packetKey(key string) (bool, tea.Cmd) {
	switch key {
	case "r":
		return true, m.toggleCapture()
	case "p":
		return true, m.checkPDHost()
	case "e":
		return true, m.exportPackets()
	case "/":
		m.previousFilter = m.view.PacketFilter
		m.view.PacketFilter = ""
		m.view.Filtering = true
		m.view.Focus = focusContent
	case "a":
		m.view.HideGoodCRC = !m.view.HideGoodCRC
		m.view.SelectedPacket = 0
	case "k":
		m.view.HideKeepAlive = !m.view.HideKeepAlive
		m.view.SelectedPacket = 0
	case " ":
		if m.opts.Capture == nil {
			if m.view.PDPaused {
				m.view.PDPaused = false
			} else {
				m.holdPackets(m.currentView(true).Packets)
			}
		}
		m.view.SelectedPacket = 0
	case "end":
		if m.opts.Capture == nil {
			m.view.PDPaused = false
		}
		m.view.SelectedPacket = 0
		m.view.PacketScroll = 0
	case "enter":
		m.view.PacketDetails = !m.view.PacketDetails
		m.view.PacketScroll = 0
	case "c":
		if m.service != nil {
			m.service.ClearPackets()
		}
		m.frozenPackets.Packets = nil
		m.view.SelectedPacket = 0
		m.notice(m.view.messages().PacketsCleared())
	case "up", "down", "pgup", "pgdown", "home":
		if m.view.PacketDetails {
			step := 1
			if key == "pgup" || key == "pgdown" {
				step = max(1, m.height-12)
			}
			if key == "up" || key == "pgup" {
				step = -step
			}
			if key == "home" {
				m.view.PacketScroll = 0
			} else {
				view := m.currentView(true)
				packets := filteredPackets(view)
				index := selectedPacketIndex(packets, m.view)
				limit := 0
				if index >= 0 {
					width, height := m.width, m.height
					if !compactScreen(width, height) {
						l := layout(width, height)
						width, height = l.mainWidth, l.bodyHeight-1
					}
					limit = max(0, len(packetDetails(packets[index], width-4, view))-max(0, height-packetHeaderRows-2))
				}
				m.view.PacketScroll = max(0, min(limit, m.view.PacketScroll+step))
			}
			return true, nil
		}
		view := m.currentView(true)
		m.holdPackets(view.Packets)
		packets := filteredPackets(view)
		index := selectedPacketIndex(packets, m.view)
		if index < 0 {
			return true, nil
		}
		step := 1
		if key == "pgup" || key == "pgdown" {
			step = max(1, m.height-14)
		}
		if key == "up" || key == "pgup" {
			step = -step
		}
		index = max(0, min(len(packets)-1, index+step))
		if key == "home" {
			index = 0
		}
		m.view.SelectedPacket = packets[index].Sequence
	default:
		return false, nil
	}
	return true, nil
}

func (m *model) mouse(msg tea.MouseMsg) tea.Cmd {
	if msg.Action != tea.MouseActionPress || msg.X < 0 || msg.Y < 0 || msg.X >= m.width || msg.Y >= m.height {
		return nil
	}
	wheel := msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown
	left := msg.Button == tea.MouseButtonLeft
	step := -1
	if msg.Button == tea.MouseButtonWheelDown {
		step = 1
	}
	if m.view.Filtering {
		if left {
			m.view.Filtering = false
		}
		return nil
	}
	if m.view.Help || m.view.Details {
		if wheel {
			m.view.Scroll = max(0, min(m.view.Scroll+step, m.scrollLimit()))
		}
		if left || msg.Button == tea.MouseButtonRight {
			m.closeOverlays()
		}
		return nil
	}
	l := layout(m.width, m.height)
	if compactScreen(m.width, m.height) {
		if m.view.Page == PagePD {
			return m.mousePackets(msg, m.width, m.height)
		}
		m.mouseOverview(msg, m.width, m.height, false)
		return nil
	}
	if wheel && (m.view.Page == PageDevice || m.view.Page == PageRecordings) {
		m.view.Scroll = max(0, min(m.view.Scroll+step, m.contentScrollLimit()))
		return nil
	}
	if !wheel && !left && msg.Button != tea.MouseButtonRight {
		return nil
	}
	if left && l.sidebar > 0 && msg.X >= 2 && msg.X < l.mainX-2 {
		if msg.Y >= 4 && msg.Y < 4+int(pageCount) {
			m.setPage(Page(msg.Y - 4))
			m.view.Focus = focusContent
			return nil
		}
	}
	if left && l.sidebar == 0 && msg.Y == 3 && msg.X >= l.mainX && msg.X < l.mainX+l.mainWidth {
		x := l.mainX
		for page := range pageCount {
			width := ansi.StringWidth(navigationStyle.Render(tabLabel(page, l.mainWidth, m.view)))
			if msg.X >= x && msg.X < x+width {
				m.setPage(page)
				m.view.Focus = focusContent
				return nil
			}
			x += width + 1
		}
	}
	if left && msg.Y == m.height-2 {
		x := l.mainX
		view := m.actionView()
		list := actions(view)
		start, end := actionWindow(list, l.mainWidth, view)
		for index := start; index < end; index++ {
			item := list[index]
			width := ansi.StringWidth(buttonStyle.Render(item.text()))
			if x+width > l.mainX+l.mainWidth {
				break
			}
			if msg.X >= x && msg.X < x+width {
				m.view.Focus = focusActions
				m.view.Action = index
				return m.mouseKey(item.key)
			}
			x += width + 1
		}
	}
	if msg.X < l.mainX || msg.X >= l.mainX+l.mainWidth || msg.Y < workspaceContentTop || msg.Y >= m.height-3 {
		return nil
	}
	msg.X -= l.mainX
	msg.Y -= workspaceContentTop
	switch m.view.Page {
	case PageOverview:
		m.mouseWorkspaceOverview(msg, l)
	case PagePD:
		return m.mousePackets(msg, l.mainWidth, l.bodyHeight-1)
	}
	return nil
}

func (m *model) mouseKey(key string) tea.Cmd {
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	*m = updated.(model)
	return cmd
}

func (m *model) mouseWorkspaceOverview(msg tea.MouseMsg, l workspaceLayout) {
	if l.dashboardOverview() {
		msg.Y += dashboardHeaderRows
		m.mouseOverview(msg, l.mainWidth, l.bodyHeight+5, true)
	} else {
		msg.Y++
		m.mouseOverview(msg, l.mainWidth, l.bodyHeight+3, false)
	}
}

func (m *model) mouseOverview(msg tea.MouseMsg, width, height int, dashboard bool) {
	chartTop, chartWidth, chartHeight := 1+overviewMetricCount, width, compactChartHeight(height)
	if dashboard {
		chartTop = dashboardChartRow
		chartWidth, chartHeight, _, _ = dashboardChartSize(width, height)
		if msg.Button == tea.MouseButtonLeft && msg.Y >= dashboardHeaderRows && msg.Y < dashboardHeaderRows+metricCardHeight {
			left := 0
			for index, cardWidth := range metricCardWidths(width) {
				if msg.X >= left && msg.X < left+cardWidth {
					m.selectMetric(index)
					return
				}
				left += cardWidth + 1
			}
		}
	} else if msg.Button == tea.MouseButtonLeft && msg.Y >= 1 && msg.Y < min(chartTop, height-1) {
		m.selectMetric(msg.Y - 1)
		return
	}
	if chartHeight < 5 || msg.Y < chartTop || msg.Y >= chartTop+chartHeight || msg.X >= chartWidth {
		return
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		m.zoomMeasurements(msg.Button == tea.MouseButtonWheelUp)
	case tea.MouseButtonRight:
		m.view.Paused = false
		m.view.Cursor = 0
	case tea.MouseButtonLeft:
		column := msg.X - 2
		row := msg.Y - chartTop
		if row >= chartHeight-1 || column < 0 || column >= chartWidth-4 {
			return
		}
		if row == 1 {
			for index, tab := range chartTabs(m.view) {
				tabWidth := ansi.StringWidth(tab)
				if column >= 0 && column < tabWidth {
					m.selectMetric(index)
					return
				}
				column -= tabWidth + len(chartTabGap)
			}
		} else if row >= 2 && row <= 2+chartPlotHeight(chartHeight) {
			m.inspectChart(column-chartAxisWidth, chartPlotColumns(chartWidth-4))
		}
	}
}

func (m *model) selectMetric(index int) {
	m.view.Metric = index
	m.view.Focus = focusContent
}

func (m *model) inspectChart(column, columns int) {
	if column < 0 || column >= columns || m.service == nil {
		return
	}
	snapshot := m.frozen
	if !m.view.Paused {
		snapshot = m.service.Snapshot(-1)
	}
	samples := historyWindow(snapshot.History, m.view.Window)
	if len(samples) == 0 {
		return
	}
	elapsed := samples[len(samples)-1].Elapsed
	target := elapsed - m.view.Window.Seconds()*(1-float64(2*column+1)/float64(2*columns-1))
	index := sort.Search(len(samples), func(index int) bool { return samples[index].Elapsed >= target })
	if index > 0 && (index == len(samples) || target-samples[index-1].Elapsed < samples[index].Elapsed-target) {
		index--
	}
	m.frozen = snapshot
	m.view.Paused = true
	m.view.Cursor = len(samples) - 1 - index
	m.view.Focus = focusContent
}

func (m *model) mousePackets(msg tea.MouseMsg, width, height int) tea.Cmd {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		_, cmd := m.packetKey("up")
		return cmd
	case tea.MouseButtonWheelDown:
		_, cmd := m.packetKey("down")
		return cmd
	case tea.MouseButtonRight:
		if m.view.PacketDetails {
			_, cmd := m.packetKey("enter")
			return cmd
		}
		return nil
	case tea.MouseButtonLeft:
	default:
		return nil
	}
	if msg.Y == 1 {
		left := 0
		for _, item := range packetFilters(width, m.view) {
			end := left + ansi.StringWidth(item.label)
			if msg.X >= left && msg.X < end {
				_, cmd := m.packetKey(item.key)
				if item.key == "/" {
					m.view.PacketFilter = m.previousFilter
				}
				m.view.Focus = focusContent
				return cmd
			}
			left = end + ansi.StringWidth(packetFilterGap)
		}
		return nil
	}
	view := m.currentView(true)
	packets := filteredPackets(view)
	selected := selectedPacketIndex(packets, view)
	if selected < 0 {
		return nil
	}
	panes := packetPanes(width, height, true, m.view.PacketDetails)
	if m.view.PacketDetails {
		if msg.Y == panes.headerHeight {
			_, cmd := m.packetKey("enter")
			return cmd
		}
		return nil
	}
	if panes.detailHeight > 0 {
		inPreview := !panes.side && msg.Y >= panes.headerHeight+panes.listHeight
		inSide := panes.side && msg.X > panes.listWidth && msg.Y >= panes.headerHeight
		if inPreview || inSide {
			m.selectPacket(view.Packets, packets[selected].Sequence, true)
			return nil
		}
	}
	if msg.X < panes.listWidth {
		start, end := packetListRange(len(packets), selected, panes.listHeight)
		row := msg.Y - panes.headerHeight - 2
		if row >= 0 && start+row < end {
			index := start + row
			m.selectPacket(view.Packets, packets[index].Sequence, index == selected)
		}
	}
	return nil
}

func (m *model) selectPacket(snapshot backend.PacketSnapshot, sequence uint64, details bool) {
	m.holdPackets(snapshot)
	m.view.SelectedPacket = sequence
	m.view.PacketDetails = details
	m.view.PacketScroll = 0
	m.view.Focus = focusContent
}
