package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/locales"
)

type Page int

const (
	PageOverview Page = iota
	PagePD
	PageRecordings
	PageDevice
	pageCount
)

const (
	focusNavigation = iota
	focusContent
	focusActions
)

var (
	surface           = lipgloss.AdaptiveColor{Light: "#F1F5F9", Dark: "#17232D"}
	activeChipSurface = lipgloss.AdaptiveColor{Light: "#E2EEFA", Dark: "#193448"}
	navigationStyle   = lipgloss.NewStyle().Padding(0, 1).Foreground(muted)
	activeStyle       = navigationStyle.Bold(true).Foreground(accent).Background(activeChipSurface)
	buttonStyle       = navigationStyle.Background(surface)
	activeButtonStyle = buttonStyle.Bold(true).Foreground(accent).Background(activeChipSurface)
	focusStyle        = buttonStyle.Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#082F49"}).Background(accent)
	selectionStyle    = lipgloss.NewStyle().Foreground(accent).Background(activeChipSurface)
)

type workspaceLayout struct{ sidebar, mainX, mainWidth, bodyHeight int }

const workspaceContentTop = 4

func (l workspaceLayout) dashboardOverview() bool {
	return l.mainWidth >= 64 && l.bodyHeight-1 >= 14
}

func layout(width, height int) workspaceLayout {
	l := workspaceLayout{mainX: 2, mainWidth: max(1, width-4), bodyHeight: max(1, height-6)}
	if width >= 108 && height >= 22 {
		l.sidebar = 24
		l.mainX += l.sidebar
		l.mainWidth -= l.sidebar
	}
	return l
}

func pageName(page Page, m locales.Messages) string {
	switch page {
	case PagePD:
		return m.PagePd()
	case PageRecordings:
		return m.PageRecordings()
	case PageDevice:
		return m.PageDevice()
	default:
		return m.PageOverview()
	}
}

const sidebarNavHeight = 2 + int(pageCount)

func navigation(s backend.Snapshot, width, height int, v View) []string {
	m := v.messages()
	var navLines []string
	for page := Page(0); page < pageCount; page++ {
		label := apart(pageName(page, m), fmt.Sprintf("F%d", page+1), width-6)
		style := navigationStyle
		if page == v.Page {
			style = activeStyle
			if v.Focus == focusNavigation {
				style = focusStyle
			}
		}
		navLines = append(navLines, style.Render(label))
	}
	navPanel := panel(strong(m.NavigationHeading(), primary), "", width, sidebarNavHeight, navLines, border)
	statusHeight := height - sidebarNavHeight - 1
	if statusHeight < 4 {
		return navPanel
	}
	rows := append(navPanel, "")
	return append(rows, statusPanel(s, width, statusHeight, v)...)
}

func statusPanel(s backend.Snapshot, width, height int, v View) []string {
	m := v.messages()
	inner := max(1, width-4)
	innerHeight := max(0, height-2)
	badge, badgeTone := connectionStatus(s, m)
	if v.SourcePath != "" {
		badge, badgeTone = m.Offline(), accent
	} else if (v.Page == PageOverview && v.Paused) || (v.Page == PagePD && v.PDPaused) {
		badge, badgeTone = m.Hold(), accent
	}
	var lines []string
	divider := borderStyle.Render(strings.Repeat("─", inner))
	if v.SourcePath != "" {
		lines = append(lines,
			apart(quietStyle.Render(m.ModeLabel()), color(m.Offline(), accent), inner),
			apart(quietStyle.Render(m.PacketList()), fmt.Sprint(len(v.Packets.Packets)), inner),
		)
		if innerHeight >= 4 {
			lines = append(lines, divider, apart(quietStyle.Render(m.BadReports()), fmt.Sprint(v.Packets.Invalid), inner))
		}
		return panel(strong(m.StatusHeading(), primary), badge, width, height, lines, border)
	}

	if s.Latest != nil {
		lines = append(lines,
			apart(quietStyle.Render(m.Voltage()), strong(fmt.Sprintf("%.4f", s.Latest.Voltage), primary)+" "+color("V", accent), inner),
			apart(quietStyle.Render(m.Current()), strong(fmt.Sprintf("%.4f", math.Abs(s.Latest.Current)), primary)+" "+color("A", positive), inner),
			apart(quietStyle.Render(m.Power()), strong(fmt.Sprintf("%.4f", s.Latest.Power), primary)+" "+color("W", warning), inner),
		)
		if s.Latest.Temperature != nil && innerHeight >= 11 {
			lines = append(lines, apart(quietStyle.Render(m.TempShort()), strong(fmt.Sprintf("%.1f", *s.Latest.Temperature), primary)+" "+color("°C", purple), inner))
		}
	} else {
		lines = append(lines, color(badge, badgeTone))
	}

	if innerHeight >= 7 {
		lines = append(lines, divider,
			apart(quietStyle.Render(m.Energy()), fmt.Sprintf("%.4f Wh", s.Statistics.WattHours), inner),
			apart(quietStyle.Render(m.ChargeShort()), fmt.Sprintf("%.3f mAh", s.Statistics.AbsoluteAmpHours*1000), inner),
		)
	}
	if innerHeight >= 9 {
		lines = append(lines, apart(quietStyle.Render(m.Measured()), duration(s.Statistics.MeasuredSeconds), inner))
	}
	if innerHeight >= 12 {
		lines = append(lines, apart(quietStyle.Render(m.Samples()), fmt.Sprint(s.Statistics.Samples), inner))
	}
	if innerHeight >= 14 {
		csvText := quietStyle.Render("○ " + m.RecordingInactive())
		if s.Recording.Active {
			csvText = strong("● "+fmt.Sprint(s.Recording.Samples), danger)
		} else if s.Recording.Path != "" {
			csvText = color("✓ "+fmt.Sprint(s.Recording.Samples), positive)
		}
		pdText := quietStyle.Render("○ " + m.RecordingInactive())
		if v.Packets.Recording.Active {
			pdText = strong("● "+fmt.Sprint(v.Packets.Recording.Packets), danger)
		} else if v.Packets.Recording.Path != "" {
			pdText = color("✓ "+fmt.Sprint(v.Packets.Recording.Packets), positive)
		}
		lines = append(lines, divider,
			apart(quietStyle.Render("CSV"), csvText, inner),
			apart(quietStyle.Render("PD"), pdText, inner),
		)
	}
	return panel(strong(m.StatusHeading(), primary), badge, width, height, lines, border)
}

func tabs(width int, v View) string {
	var labels []string
	for page := Page(0); page < pageCount; page++ {
		style := navigationStyle
		if page == v.Page {
			style = activeStyle
			if v.Focus == focusNavigation {
				style = focusStyle
			}
		}
		labels = append(labels, style.Render(tabLabel(page, width, v)))
	}
	return fit(strings.Join(labels, " "), width)
}

func tabLabel(page Page, width int, v View) string {
	label := fmt.Sprintf("F%d", page+1)
	if width >= 70 || page == v.Page {
		label += " " + pageName(page, v.messages())
	}
	return label
}

func workspace(s backend.Snapshot, w, h int, v View) []string {
	l := layout(w, h)
	m := v.messages()
	state, tone := connectionStatus(s, m)
	if v.SourcePath != "" {
		state, tone = m.Offline(), accent
	}
	var badges []string
	if s.Recording.Active || v.Packets.Recording.Active {
		badges = append(badges, strong("● "+m.RecBadge(), danger))
	}
	if (v.Page == PageOverview && v.Paused) || (v.Page == PagePD && v.PDPaused && v.SourcePath == "") {
		badges = append(badges, strong("Ⅱ "+m.Hold(), accent))
	}
	badges = append(badges, strong("● "+state, tone))
	brand := strong("WATTSCOPE", accent)
	if w >= 70 {
		brand += quietStyle.Render("  /  " + m.Workspace())
	}
	header := apart(brand, strings.Join(badges, "  "), w-4)
	var body []string
	if l.sidebar > 0 {
		name := safe(s.Device.Name)
		if s.Device.Simulated {
			name = m.SimulatedMeter()
		}
		if v.SourcePath != "" {
			name = m.OfflineControls()
		} else if s.Device.Serial != "" {
			if name != "" {
				name += " · " + safe(s.Device.Serial)
			} else {
				name = safe(s.Device.Serial)
			}
		}
		body = append(body, apart(strong(pageName(v.Page, m), primary), quietStyle.Render(name), l.mainWidth))
	} else {
		body = append(body, tabs(l.mainWidth, v))
	}
	contentHeight := l.bodyHeight - 1
	var content []string
	switch v.Page {
	case PagePD:
		content = packetsPage(s, l.mainWidth, contentHeight, v)
	case PageRecordings:
		content = recordingsPage(s, l.mainWidth, contentHeight, v)
	case PageDevice:
		rows := details(s, v, l.mainWidth+4)
		offset := max(0, min(v.Scroll, max(0, len(rows)-max(0, contentHeight-2))))
		content = panel(strong(m.DeviceStorage(), primary), m.ScrollHint(), l.mainWidth, contentHeight, rows[offset:], border)
	default:
		if !l.dashboardOverview() {
			rows := compact(s, l.mainWidth, contentHeight+4, v)
			content = rows[1 : len(rows)-3]
		} else {
			rows := dashboard(s, l.mainWidth, contentHeight+6, v)
			content = rows[dashboardHeaderRows : len(rows)-3]
		}
	}
	body = append(body, content...)
	body = strings.Split(screen(body, l.mainWidth, l.bodyHeight), "\n")
	if l.sidebar > 0 {
		body = beside(navigation(s, l.sidebar-2, l.bodyHeight, v), body, 2)
	}
	rows := []string{"", "  " + header, "  " + borderStyle.Render(strings.Repeat("─", max(0, w-4)))}
	for _, line := range body {
		rows = append(rows, "  "+line)
	}
	status := statusLine(s, l.mainWidth, v)
	if v.Page == PageOverview && s.Latest != nil && s.Error == "" && v.Message == "" {
		status = quietStyle.Render(m.HostTotals(fmt.Sprintf("%.4f", s.Statistics.WattHours), fmt.Sprintf("%.3f", s.Statistics.AbsoluteAmpHours*1000)))
		if l.mainWidth >= 70 {
			status = apart(status, quietStyle.Render(m.Quality(fmt.Sprint(s.Statistics.Samples), fmt.Sprint(s.InvalidReports))), l.mainWidth)
		}
	}
	if v.Page == PagePD || v.SourcePath != "" {
		if s.Error == "" {
			status = quietStyle.Render(m.PacketRetention(fmt.Sprint(len(v.Packets.Packets)), fmt.Sprint(v.Packets.Evicted), fmt.Sprint(v.Packets.Invalid)))
		}
		if v.Message != "" && s.Error == "" {
			status = color(safe(v.Message), accent)
		}
	}
	rows = append(rows, strings.Repeat(" ", l.mainX)+fit(status, l.mainWidth), strings.Repeat(" ", l.mainX)+actionBar(l.mainWidth, v), "")
	return rows
}

func recordingsPage(s backend.Snapshot, w, h int, v View) []string {
	rows := recordingsRows(s, w, v)
	offset := max(0, min(v.Scroll, max(0, len(rows)-max(0, h-2))))
	return panel(strong(v.messages().PageRecordings(), primary), v.messages().ScrollHint(), w, h, rows[offset:], border)
}

func recordingsRows(s backend.Snapshot, w int, v View) []string {
	m := v.messages()
	rows := []string{quietStyle.Render(m.RecordsDescription()), ""}
	add := func(title, path string, active bool, countText string) {
		badge := "○ " + m.RecordingInactive()
		tone := muted
		if active {
			badge = "● " + m.Recording()
			tone = danger
		} else if path != "" {
			badge = "✓ " + m.RecordingSaved()
			tone = positive
		}
		rows = append(rows, apart(strong(title, accent), color(badge, tone), w-4))
		if path != "" {
			rows = append(rows, quietStyle.Render(countText), middle(safe(path), w-4), "")
		} else {
			rows = append(rows, quietStyle.Render(m.RecordingNotStarted()), "")
		}
	}
	add(m.RecordCsv(), s.Recording.Path, s.Recording.Active, m.Rows(int(s.Recording.Samples)))
	add(m.RecordPd(), v.Packets.Recording.Path, v.Packets.Recording.Active, m.PacketsCount(int(v.Packets.Recording.Packets)))
	rows = append(rows, quietStyle.Render(m.RecordingsDirectory()), middle(safe(v.RecordingDir), w-4))
	if v.SourcePath != "" {
		rows = append(rows, "", quietStyle.Render(m.SourceFile()), middle(safe(v.SourcePath), w-4), quietStyle.Render(m.OfflineControls()))
	} else {
		rows = append(rows, "", quietStyle.Render(m.RecordControls()))
	}
	return rows
}
