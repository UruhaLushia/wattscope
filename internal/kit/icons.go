package kit

import (
	appicon "github.com/UruhaLushia/wattscope/assets/icon"
	"github.com/egoist/mygo/ui"
)

func svgIcon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

func brandMarkSVG(track, fg string) *ui.SVG {
	return ui.MustParseSVG([]byte(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="156 156 712 712">` +
			`<path d="M262 682 A290 290 0 1 1 762 682" fill="none" stroke="` + track + `" stroke-width="68" stroke-linecap="round"/>` +
			`<path d="M262 682 A290 290 0 0 1 657 284" fill="none" stroke="` + fg + `" stroke-width="68" stroke-linecap="round"/>` +
			`<path d="M556 340 L408 568 H510 L478 740 L628 506 H526 Z" fill="` + fg + `" stroke="` + fg + `" stroke-width="28" stroke-linejoin="round"/>` +
			`</svg>`,
	))
}

// BrandMark returns the transparent vector brand mark tailored for light or dark canvases.
func BrandMark(dark bool) *ui.SVG {
	if dark {
		return brandMarkDark
	}
	return brandMarkLight
}

// BrandLogo and Icon* SVG assets provide WattScope's vector icons.
var (
	BrandLogo      = ui.MustParseSVG(appicon.SVG)
	brandMarkLight = brandMarkSVG("#d5dff8", "#3e63dd")
	brandMarkDark  = brandMarkSVG("#2c3752", "#6e92ff")

	// Navigation & brand
	IconActivity  = svgIcon(`<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>`)
	IconUSB       = svgIcon(`<circle cx="10" cy="7" r="1"/><circle cx="4" cy="20" r="1"/><path d="M4.7 19.3 19 5"/><path d="m21 3-3 1 2 2Z"/><path d="M9.26 7.68 5 12l2 5"/><path d="m10 14 5 2 3.5-3.5"/><path d="m18 12 1-1 1 1-1 1Z"/>`)
	IconFileText  = svgIcon(`<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/>`)
	IconCpu       = svgIcon(`<rect x="4" y="4" width="16" height="16" rx="2"/><rect x="9" y="9" width="6" height="6"/><path d="M15 2v2"/><path d="M15 20v2"/><path d="M2 15h2"/><path d="M2 9h2"/><path d="M20 15h2"/><path d="M20 9h2"/><path d="M9 2v2"/><path d="M9 20v2"/>`)
	IconSliders   = svgIcon(`<path d="M4 7h9"/><path d="M17 7h3"/><circle cx="15" cy="7" r="2"/><path d="M4 17h4"/><path d="M12 17h8"/><circle cx="10" cy="17" r="2"/>`)
	IconTopBar    = svgIcon(`<rect x="3" y="4" width="18" height="16" rx="3.5"/><path d="M8.5 8h7" stroke-width="2.2"/>`)
	IconBottomBar = svgIcon(`<rect x="3" y="4" width="18" height="16" rx="3.5"/><path d="M7.5 16h9" stroke-width="2.2"/>`)
	IconSidebar   = svgIcon(`<rect x="3" y="4" width="18" height="16" rx="3.5"/><rect x="5" y="6" width="4.5" height="12" rx="1.4" fill="currentColor" stroke="none"/>`)
	IconLayers    = svgIcon(`<path d="m12 3 9 5-9 5-9-5 9-5ZM3 12l9 5 9-5M3 16l9 5 9-5"/>`)
	IconGlobe     = svgIcon(`<circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a14 14 0 0 1 0 18"/><path d="M12 3a14 14 0 0 0 0 18"/>`)
	IconPalette   = svgIcon(`<circle cx="13.5" cy="6.5" r=".5" fill="currentColor"/><circle cx="17.5" cy="10.5" r=".5" fill="currentColor"/><circle cx="8.5" cy="7.5" r=".5" fill="currentColor"/><circle cx="6.5" cy="12.5" r=".5" fill="currentColor"/><path d="M12 2C6.5 2 2 6.5 2 12s4.5 10 10 10c.926 0 1.648-.746 1.648-1.688 0-.437-.18-.835-.437-1.125-.29-.289-.438-.652-.438-1.125a1.64 1.64 0 0 1 1.668-1.668h1.996c3.051 0 5.555-2.503 5.555-5.554C21.965 6.012 17.461 2 12 2z"/>`)
	IconShield    = svgIcon(`<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/>`)

	// Title bar & window controls
	IconMoon     = svgIcon(`<path d="M20 14.5A8.5 8.5 0 0 1 9.5 4a8.5 8.5 0 1 0 10.5 10.5z"/>`)
	IconSun      = svgIcon(`<circle cx="12" cy="12" r="4"/><path d="M12 2.5v2M12 19.5v2M2.5 12h2M19.5 12h2M5.2 5.2l1.4 1.4M17.4 17.4l1.4 1.4M18.8 5.2l-1.4 1.4M6.6 17.4l-1.4 1.4"/>`)
	IconMinimize = svgIcon(`<path d="M5 12h14"/>`)
	IconMaximize = svgIcon(`<rect x="5.5" y="5.5" width="13" height="13" rx="2"/>`)
	IconRestore  = svgIcon(`<rect x="4.5" y="7.5" width="10.5" height="10.5" rx="1.8"/><path d="M8.5 4.5h9a2 2 0 0 1 2 2v9"/>`)
	IconClose    = svgIcon(`<path d="M6 6l12 12M18 6 6 18"/>`)

	// Actions & status
	IconPlay           = svgIcon(`<path d="M8 5.2 19 12 8 18.8z" fill="currentColor" stroke="none"/>`)
	IconPause          = svgIcon(`<rect x="6" y="5" width="4" height="14" rx="1" fill="currentColor" stroke="none"/><rect x="14" y="5" width="4" height="14" rx="1" fill="currentColor" stroke="none"/>`)
	IconStop           = svgIcon(`<rect x="6" y="6" width="12" height="12" rx="2" fill="currentColor" stroke="none"/>`)
	IconRecord         = svgIcon(`<circle cx="12" cy="12" r="5" fill="currentColor" stroke="none"/>`)
	IconRefresh        = svgIcon(`<path d="M20 7v5h-5M4 17v-5h5"/><path d="M6.1 7a7 7 0 0 1 11.5-1.6L20 8M4 16l2.4 2.6A7 7 0 0 0 17.9 17"/>`)
	IconSearch         = svgIcon(`<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4 4"/>`)
	IconFilter         = svgIcon(`<path d="M3 6h18"/><path d="M7 12h10"/><path d="M10 18h4"/>`)
	IconCopy           = svgIcon(`<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>`)
	IconFolder         = svgIcon(`<path d="M3 7V5a1 1 0 0 1 1-1h5l2 3h9a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V7Z"/>`)
	IconDownload       = svgIcon(`<path d="M12 3v11m-4-4 4 4 4-4"/><path d="M4 15v5h16v-5"/>`)
	IconTrash          = svgIcon(`<path d="M3 6h18M9 6V4h6v2M5 6l1 15h12l1-15M10 10v7M14 10v7"/>`)
	IconCheck          = svgIcon(`<path d="M4.5 12.5 9.5 17.5 19.5 6.5"/>`)
	IconChevronDown    = svgIcon(`<path d="m6 9 6 6 6-6"/>`)
	IconChevronsUpDown = svgIcon(`<path d="m7 15 5 5 5-5"/><path d="m7 9 5-5 5 5"/>`)
	IconChevronL       = svgIcon(`<path d="m15 5-7 7 7 7"/>`)
	IconChevronR       = svgIcon(`<path d="m9 5 7 7-7 7"/>`)
	IconArrowLeft      = svgIcon(`<path d="M20 12H4m6-6-6 6 6 6"/>`)
	IconArrowRight     = svgIcon(`<path d="M4 12h16m-6-6 6 6-6 6"/>`)
	IconArrowDown      = svgIcon(`<path d="M12 4v16m-6-6 6 6 6-6"/>`)
	IconArrowUp        = svgIcon(`<path d="M12 20V4m-6 6 6-6 6 6"/>`)
	IconHome           = svgIcon(`<path d="m3 10 9-7 9 7M5 9v11h5v-7h4v7h5V9"/>`)
	IconEdit           = svgIcon(`<path d="m15 5 4 4M4 20l4-1L20 7a2.8 2.8 0 0 0-4-4L4 15z"/>`)
	IconMore           = svgIcon(`<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>`)
	IconZap            = svgIcon(`<path d="M13 2 4 14h7l-1 8 9-12h-7l1-8z"/>`)
)
