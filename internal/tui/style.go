package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	accent      = lipgloss.AdaptiveColor{Light: "#1479CE", Dark: "#57C5FF"}
	positive    = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	warning     = lipgloss.AdaptiveColor{Light: "#A16207", Dark: "#FBBF24"}
	danger      = lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#F87171"}
	purple      = lipgloss.AdaptiveColor{Light: "#7E22CE", Dark: "#C084FC"}
	muted       = lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#94A3B8"}
	border      = lipgloss.AdaptiveColor{Light: "#CBD5E1", Dark: "#344D60"}
	primary     = lipgloss.AdaptiveColor{Light: "#0F172A", Dark: "#E2E8F0"}
	quietStyle  = lipgloss.NewStyle().Foreground(muted)
	borderStyle = lipgloss.NewStyle().Foreground(border)
)

func color(text string, c lipgloss.TerminalColor) string {
	return lipgloss.NewStyle().Foreground(c).Render(text)
}
func strong(text string, c lipgloss.TerminalColor) string {
	return lipgloss.NewStyle().Bold(true).Foreground(c).Render(text)
}

func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

func fit(text string, width int) string {
	width = max(0, width)
	text = ansi.Truncate(text, width, "")
	return text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}

func fitRight(text string, width int) string {
	width = max(0, width)
	text = ansi.Truncate(text, width, "")
	return strings.Repeat(" ", max(0, width-ansi.StringWidth(text))) + text
}

func apart(left, right string, width int) string {
	if ansi.StringWidth(right)+2 >= width {
		return fit(left, width)
	}
	return fit(left, width-ansi.StringWidth(right)) + right
}

func middle(text string, width int) string {
	if ansi.StringWidth(text) <= width {
		return text
	}
	if width < 5 {
		return ansi.Truncate(text, max(0, width), "")
	}
	left := (width - 1) / 3
	return ansi.Cut(text, 0, left) + "…" + ansi.TruncateLeft(text, ansi.StringWidth(text)-(width-left-1), "")
}

func panel(title, badge string, width, height int, content []string, tone lipgloss.TerminalColor) []string {
	width = max(4, width)
	height = max(2, height)
	line := func(s string) string { return color(s, tone) }
	inner := width - 2
	var top string
	if badge != "" {
		right := " " + quietStyle.Render(badge) + " " + line("─")
		rightW := ansi.StringWidth(right)
		maxLeftW := inner - rightW - 1
		if maxLeftW >= 6 {
			t := ansi.Truncate(title, maxLeftW-3, "…")
			left := line("─") + " " + t + " "
			fill := max(1, inner-ansi.StringWidth(left)-rightW)
			top = line("╭") + left + line(strings.Repeat("─", fill)) + right + line("╮")
		}
	}
	if top == "" {
		maxTitle := max(0, inner-3)
		t := ansi.Truncate(title, maxTitle, "…")
		left := line("─") + " " + t + " "
		if maxTitle == 0 {
			left = ""
		}
		fill := max(0, inner-ansi.StringWidth(left))
		top = line("╭") + left + line(strings.Repeat("─", fill)) + line("╮")
	}
	rows := []string{top}
	for i := 0; i < height-2; i++ {
		text := ""
		if i < len(content) {
			text = content[i]
		}
		rows = append(rows, line("│")+" "+fit(text, width-4)+" "+line("│"))
	}
	return append(rows, line("╰"+strings.Repeat("─", width-2)+"╯"))
}

func beside(left, right []string, gap int) []string {
	rows := make([]string, max(len(left), len(right)))
	lw := 0
	if len(left) > 0 {
		lw = ansi.StringWidth(left[0])
	}
	for i := range rows {
		a, b := "", ""
		if i < len(left) {
			a = left[i]
		}
		if i < len(right) {
			b = right[i]
		}
		rows[i] = fit(a, lw) + strings.Repeat(" ", gap) + b
	}
	return rows
}

func screen(rows []string, width, height int) string {
	width = max(1, width)
	height = max(1, height)
	if len(rows) > height {
		rows = append(rows[:height-1], rows[len(rows)-1])
	}
	for i := range rows {
		rows[i] = fit(rows[i], width)
	}
	for len(rows) < height {
		rows = append(rows, strings.Repeat(" ", width))
	}
	return strings.Join(rows, "\n")
}

func duration(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	return (time.Duration(seconds) * time.Second).String()
}

func span(d time.Duration) string {
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", d/time.Hour)
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return d.String()
}
