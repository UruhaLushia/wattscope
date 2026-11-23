package kit

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	motionFastDur         = 120 * time.Millisecond
	motionNavDur          = 175 * time.Millisecond
	motionMoveDur         = 220 * time.Millisecond
	motionPopupOpenDur    = 180 * time.Millisecond
	motionPopupCloseDur   = 110 * time.Millisecond
	motionOptionCloseDur  = 125 * time.Millisecond
	motionPopupDur        = 230 * time.Millisecond
	motionSheetDur        = 260 * time.Millisecond
	motionPageDur         = 235 * time.Millisecond
	motionUnfoldDur       = 280 * time.Millisecond
	rotateWindowDur       = 380 * time.Millisecond
	navLayoutSwitchDur    = 520 * time.Millisecond
	navLayoutSwitchDurCPU = 220 * time.Millisecond
	pageVeilDur           = 140 * time.Millisecond
)

const (
	backendUnknown     uint32 = 0
	backendAccelerated uint32 = 1
	backendSoftware    uint32 = 2
)

var (
	easeStandard   = ui.CubicBezier(0.2, 0, 0, 1)
	easeAccelerate = ui.CubicBezier(0.4, 0, 1, 1)

	// MotionFast is the short micro-interaction animation for hover, press, and color fades.
	MotionFast         = ui.Animation{Duration: motionFastDur, Ease: ui.EaseOut}
	motionNav          = ui.Animation{Duration: motionNavDur, Ease: easeStandard}
	motionChipOut      = ui.Animation{Duration: time.Nanosecond}
	motionChipIn       = ui.Animation{Delay: motionOptionCloseDur, Duration: 75 * time.Millisecond, Ease: easeStandard}
	motionCapsuleIn    = ui.Animation{Duration: 120 * time.Millisecond, Ease: ui.EaseOut}
	motionCapsuleDrag  = ui.Animation{Duration: 22 * time.Millisecond, Ease: ui.EaseOut}
	motionMove         = ui.Animation{Duration: motionMoveDur, Ease: ui.EaseOut}
	motionLocale       = ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.04}}
	motionPage         = ui.Animation{Spring: ui.SpringOptions{Duration: motionPageDur, Bounce: 0.03}}
	motionGlassPress   = ui.Animation{Spring: ui.SpringOptions{Duration: 115 * time.Millisecond, Bounce: 0.24}}
	motionGlassRelease = ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond, Bounce: 0.28}}
	motionGlassScrub   = ui.Animation{Spring: ui.SpringOptions{Duration: 180 * time.Millisecond, Bounce: 0.22}}
	motionGlassMorph   = ui.Animation{Duration: 240 * time.Millisecond, Ease: easeStandard}
	motionGlassShape   = ui.Animation{Spring: ui.SpringOptions{Duration: 280 * time.Millisecond, Bounce: 0.08}}
	motionNavPanel     = ui.Animation{Spring: ui.SpringOptions{Duration: 340 * time.Millisecond, Bounce: 0.14}}
	motionNavPanelCPU  = ui.Animation{Duration: 200 * time.Millisecond, Ease: easeStandard}
	// MotionRotate is the spring animation used when elements transition across mobile orientations.
	MotionRotate       = ui.Animation{Spring: ui.SpringOptions{Duration: 320 * time.Millisecond, Bounce: 0.04}}
	motionRotatePanel  = ui.Animation{Spring: ui.SpringOptions{Duration: 290 * time.Millisecond, Bounce: 0.05}}
	motionSpring       = ui.Animation{Spring: ui.SpringOptions{Duration: motionMoveDur, Bounce: 0.06}}
	motionOptionSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 230 * time.Millisecond, Bounce: 0.06}}
	motionPopup        = ui.Animation{Duration: motionPopupOpenDur, Ease: easeStandard}
	motionPopupCPU     = ui.Animation{Duration: 150 * time.Millisecond, Ease: easeStandard}
	motionPopupSpring  = ui.Animation{Spring: ui.SpringOptions{Duration: 220 * time.Millisecond, Bounce: 0.09}}
	motionDialogPop    = ui.Animation{Spring: ui.SpringOptions{Duration: motionPopupDur, Bounce: 0.06}}
	motionDialogCPU    = ui.Animation{Duration: 160 * time.Millisecond, Ease: easeStandard}
	motionSheet        = ui.Animation{Spring: ui.SpringOptions{Duration: motionSheetDur, Bounce: 0.04}}

	localeTextTransition = ui.ContentTransition{Morph: true, Animation: &motionLocale}
	localeSizeTransition = ui.ElementTransition{Position: true, Size: true, Animation: &motionLocale}

	layoutTransition = ui.ElementTransition{Position: true, Size: true, Animation: &motionMove}
	// SideContentTransition animates content shifts beside a toggling navigation rail.
	SideContentTransition = ui.ElementTransition{
		Position:  true,
		Size:      true,
		Animation: &motionNavPanel,
	}
	sideContentTransitionCPU = ui.ElementTransition{
		Position:  true,
		Size:      true,
		Animation: &motionNavPanelCPU,
	}
	popupTransition = ui.ElementTransition{
		Animation:    &motionPopupSpring,
		ExitDuration: motionPopupCloseDur,
		ExitEase:     easeAccelerate,
		Position:     true,
		Enter:        &ui.Motion{Y: -3, Scale: 0.90, Flip: true},
		Exit:         &ui.Motion{Y: -1, Scale: 0.94, Flip: true},
	}
	popupTransitionCPU = ui.ElementTransition{
		Animation:    &motionPopupCPU,
		ExitDuration: motionPopupCloseDur,
		ExitEase:     easeAccelerate,
		Position:     true,
		Enter:        &ui.Motion{Y: -3, Scale: 0.925, Flip: true},
		Exit:         &ui.Motion{Y: -1, Scale: 0.94, Flip: true},
	}
	optionPopupTransition = ui.ElementTransition{
		Animation:    &motionOptionSpring,
		ExitDuration: motionOptionCloseDur,
		ExitEase:     easeStandard,
		Enter:        &ui.Motion{Scale: 0.78, Opacity: 1},
		Exit:         &ui.Motion{Scale: 0.88, Opacity: 0},
	}
	optionPopupTransitionCPU = ui.ElementTransition{
		Animation:    &motionPopupCPU,
		ExitDuration: motionOptionCloseDur,
		ExitEase:     easeStandard,
		Position:     true,
		Enter:        &ui.Motion{Scale: 0.925, Opacity: 1},
		Exit:         &ui.Motion{Scale: 0.925, Opacity: 0},
	}
	modalTransition = ui.ElementTransition{
		Position: true, Colors: true, Animation: &motionDialogPop,
		Enter: &ui.Motion{Y: 10, Scale: 0.94},
		Exit:  &ui.Motion{Y: 6, Scale: 0.96},
	}
	modalTransitionCPU = ui.ElementTransition{
		Position: true, Colors: true, Animation: &motionDialogCPU,
		Enter: &ui.Motion{Y: 10, Scale: 0.94},
		Exit:  &ui.Motion{Y: 6, Scale: 0.96},
	}
	sheetTransition = ui.ElementTransition{
		Position: true, Colors: true, Animation: &motionSheet,
		Enter: &ui.Motion{Y: 56},
		Exit:  &ui.Motion{Y: 40},
	}
	sheetTransitionCPU = ui.ElementTransition{
		Position: true, Colors: true, Animation: &motionDialogCPU,
		Enter: &ui.Motion{Y: 48},
		Exit:  &ui.Motion{Y: 40},
	}
	backdropTransition = ui.ElementTransition{
		Animation: &motionMove,
		Colors:    true,
		Enter:     &ui.Motion{},
		Exit:      &ui.Motion{},
	}

	motionSection = ui.Animation{Spring: ui.SpringOptions{Duration: 240 * time.Millisecond, Bounce: 0.04}}
	motionInline  = ui.Animation{Spring: ui.SpringOptions{Duration: 210 * time.Millisecond, Bounce: 0.06}}

	sectionTransition = ui.ElementTransition{
		Position:     true,
		Size:         true,
		Animation:    &motionSection,
		ExitDuration: 130 * time.Millisecond,
		ExitEase:     easeAccelerate,
		Enter:        &ui.Motion{Y: 8, Scale: 0.985, Opacity: 0},
		Exit:         &ui.Motion{Y: -4, Scale: 0.985, Opacity: 0},
	}
	inlineTransition = ui.ElementTransition{
		Position:     true,
		Size:         true,
		Animation:    &motionInline,
		ExitDuration: 110 * time.Millisecond,
		ExitEase:     easeAccelerate,
		Enter:        &ui.Motion{Scale: 0.88, Opacity: 0},
		Exit:         &ui.Motion{Scale: 0.90, Opacity: 0},
	}
)

type frameLayoutState struct {
	initialized              bool
	w, h                     float32
	top, right, bottom, left float32
	until                    time.Time
	rotateUntil              time.Time
	rotateGen                uint64
	navSidebarInit           bool
	navSidebar               bool
	navSwitchUntil           time.Time
	pageVeilInit             bool
	pageVeilPage             int
	pageVeilStart            time.Time
	services                 ui.Services
	accelState               *atomic.Uint32
	probeDraw                func(*ui.Painter, ui.Rect)
}

func (s *frameLayoutState) recordPainterBackend(p *ui.Painter, _ ui.Rect) {
	next := backendSoftware
	if p.Accelerated() {
		next = backendAccelerated
	}
	if s.accelState == nil {
		s.accelState = new(atomic.Uint32)
	}
	prev := s.accelState.Swap(next)
	if prev != next && s.services != (ui.Services{}) {
		s.services.Invalidate()
	}
}

type frameMotionState struct {
	scaleReflow          bool
	staticSnapshot       bool
	orientationRotating  bool
	software             bool
	orientationRotateGen uint64
	layout               *frameLayoutState
}

var (
	frameStatesMu sync.RWMutex
	frameStates   = map[ui.Services]*frameMotionState{}
)

func readFrameMotionState(ctx *ui.Context) frameMotionState {
	if ctx == nil {
		return frameMotionState{}
	}
	key := ctx.Services()
	frameStatesMu.RLock()
	st := frameStates[key]
	var out frameMotionState
	if st != nil {
		out = *st
	}
	frameStatesMu.RUnlock()
	return out
}

// IsSoftware reports whether the window painter runs on the software CPU rasterizer.
func IsSoftware(ctx *ui.Context) bool {
	return readFrameMotionState(ctx).software
}

// IsOrientationRotating reports whether ctx is within a mobile orientation transition window.
func IsOrientationRotating(ctx *ui.Context) bool {
	return readFrameMotionState(ctx).orientationRotating
}

func isStaticSnapshot(ctx *ui.Context) bool {
	return readFrameMotionState(ctx).staticSnapshot
}

func isScaleReflow(ctx *ui.Context) bool {
	return readFrameMotionState(ctx).scaleReflow
}

// DesktopNavSwitchTracker reports whether a desktop navigation layout switch
// is still in progress during Paint/Draw callbacks after BeginFrame returns.
type DesktopNavSwitchTracker struct {
	state *frameLayoutState
}

// DesktopNavSwitch captures the current window's persistent layout switch
// tracker during build for lock-safe queries during Paint/Draw.
func DesktopNavSwitch(ctx *ui.Context) DesktopNavSwitchTracker {
	fs := readFrameMotionState(ctx)
	if fs.staticSnapshot {
		return DesktopNavSwitchTracker{}
	}
	return DesktopNavSwitchTracker{state: fs.layout}
}

// Active reports whether a desktop navigation layout switch is still active at now.
func (t DesktopNavSwitchTracker) Active(now time.Time) bool {
	if t.state == nil {
		return false
	}
	frameStatesMu.RLock()
	until := t.state.navSwitchUntil
	frameStatesMu.RUnlock()
	return now.Before(until)
}

func trackDesktopNavLayoutSwitch(ctx *ui.Context, sidebar bool) bool {
	if ctx == nil {
		return false
	}
	key := ctx.Services()
	now := ctx.Now()
	frameStatesMu.Lock()
	st := frameStates[key]
	if st == nil || st.staticSnapshot || st.layout == nil {
		frameStatesMu.Unlock()
		return false
	}
	ls := st.layout
	if !ls.navSidebarInit {
		ls.navSidebarInit, ls.navSidebar = true, sidebar
		frameStatesMu.Unlock()
		return false
	}
	if ls.navSidebar != sidebar {
		dur := navLayoutSwitchDur
		if st.software {
			dur = navLayoutSwitchDurCPU
		}
		ls.navSidebar = sidebar
		ls.navSwitchUntil = now.Add(dur)
		frameStatesMu.Unlock()
		ctx.After(dur)
		return true
	}
	if now.Before(ls.navSwitchUntil) {
		rem := ls.navSwitchUntil.Sub(now)
		frameStatesMu.Unlock()
		ctx.After(rem)
		return true
	}
	frameStatesMu.Unlock()
	return false
}

// BeginStaticSnapshot suppresses shared Match keys and entry/layout transitions
// while painting an off-frame snapshot (such as the previous theme reveal layer).
func BeginStaticSnapshot(ctx *ui.Context) func() {
	if ctx == nil {
		return func() {}
	}
	key := ctx.Services()
	frameStatesMu.Lock()
	st, existed := frameStates[key]
	var prev bool
	if existed {
		prev = st.staticSnapshot
		st.staticSnapshot = true
	} else {
		frameStates[key] = &frameMotionState{staticSnapshot: true}
	}
	frameStatesMu.Unlock()
	return func() {
		frameStatesMu.Lock()
		if !existed {
			delete(frameStates, key)
		} else if cur := frameStates[key]; cur != nil {
			cur.staticSnapshot = prev
		}
		frameStatesMu.Unlock()
	}
}

func glassPressAnimation(pressed bool) ui.Animation {
	if pressed {
		return motionGlassPress
	}
	return motionGlassRelease
}

// Desktop navigation shares one spring so its tabs and toggle stay with the panel.
func navMatchTransition(ctx *ui.Context, matchKey any) ui.ElementTransition {
	fs := readFrameMotionState(ctx)
	if fs.staticSnapshot {
		return ui.ElementTransition{}
	}
	if fs.software {
		return ui.ElementTransition{
			Position:  true,
			Size:      true,
			Animation: &motionNavPanelCPU,
			Match:     matchKey,
			Fly:       false,
		}
	}
	return ui.ElementTransition{
		Position:  true,
		Size:      true,
		Animation: &motionNavPanel,
		Match:     matchKey,
		Fly:       true,
	}
}

// NavRotateMatchTransition returns a matched-geometry transition for mobile
// elements whose layout moves across portrait <-> landscape rotations while
// snapping inset or same-orientation updates immediately.
func NavRotateMatchTransition(ctx *ui.Context, matchKey any) ui.ElementTransition {
	fs := readFrameMotionState(ctx)
	if fs.staticSnapshot {
		return ui.ElementTransition{}
	}
	return ui.ElementTransition{
		Position:  fs.orientationRotating,
		Size:      fs.orientationRotating,
		Colors:    true,
		Shadows:   false,
		Animation: &MotionRotate,
		Match:     matchKey,
		Fly:       true,
	}
}

// ApplyRotateEnterPose applies a GPU-composited entrance pose when el mounts
// or re-orients during a mobile orientation flip, keeping transitions smooth
// even though mygo/ui suppresses ElementTransition animations on window resizes.
func ApplyRotateEnterPose(ctx *ui.Context, el ui.Element, key any, start ui.Pose, anim ui.Animation) ui.Element {
	if ctx == nil {
		return el
	}
	fs := readFrameMotionState(ctx)
	if fs.staticSnapshot {
		return el
	}
	type rotEnterState struct {
		gen       uint64
		startedAt time.Time
	}
	now := ctx.Now()
	st := ui.Local(el, key, func() rotEnterState { return rotEnterState{} })
	pose := ui.Pose{}
	if fs.orientationRotating && fs.orientationRotateGen != 0 && st.gen != fs.orientationRotateGen {
		st.gen = fs.orientationRotateGen
		st.startedAt = now
	}
	if !fs.orientationRotating && (st.gen == 0 || now.Sub(st.startedAt) > rotateWindowDur+200*time.Millisecond) {
		return el
	}
	// Hold the start pose through any multi-pass rebuild or same-timestamp inset settle on frame 0, then release 1ms later.
	if st.gen != 0 && !now.After(st.startedAt) {
		pose = start
		ctx.After(time.Millisecond)
	}
	return el.AnimatePose(pose, anim)
}

// BeginFrame prepares the overlay tree, enables matched orientation transitions
// when a mobile viewport rotates, and suppresses positional transitions on
// desktop window resizes or same-orientation safe-area inset updates.
func BeginFrame(ctx *ui.Context, mobile bool) func() {
	ui.Overlay(ctx, func() {})
	w, h := ctx.Size()
	top, right, bottom, left := ctx.SafeArea()
	key := ctx.Services()
	probe := ui.Box(ctx).Absolute().Size(0, 0).PassThrough()
	state := ui.Local(probe, "frame-layout", func() frameLayoutState {
		return frameLayoutState{accelState: new(atomic.Uint32)}
	})
	if state.probeDraw == nil {
		state.services = key
		state.probeDraw = state.recordPainterBackend
	}
	probe.Draw(state.probeDraw)
	software := state.accelState.Load() == backendSoftware
	now := ctx.Now()
	insetSnap := false
	resetNavSwitch := false
	if w > 0 && h > 0 {
		switch {
		case !state.initialized:
			state.initialized = true
			state.w, state.h = w, h
			state.top, state.right, state.bottom, state.left = top, right, bottom, left
		case state.w != w || state.h != h:
			orientationFlip := mobile && ((state.w > state.h) != (w > h))
			sameSideSettling := mobile && !orientationFlip && now.Before(state.rotateUntil)
			state.w, state.h = w, h
			state.top, state.right, state.bottom, state.left = top, right, bottom, left
			switch {
			case orientationFlip:
				state.until = time.Time{}
				state.rotateGen++
				state.rotateUntil = now.Add(rotateWindowDur)
				resetNavSwitch = true
			case sameSideSettling:
				// Preserve ongoing rotation springs if Android finishes bar/cutout sizing a frame later.
			default:
				state.until = now.Add(motionUnfoldDur)
				state.rotateUntil = time.Time{}
				resetNavSwitch = true
			}
		case mobile && (state.top != top || state.right != right || state.bottom != bottom || state.left != left):
			state.top, state.right, state.bottom, state.left = top, right, bottom, left
			if !now.Before(state.rotateUntil) {
				state.rotateUntil = time.Time{}
				insetSnap = true
				resetNavSwitch = true
			}
		}
	}
	scaleReflow := insetSnap || now.Before(state.until)
	orientationRotating := mobile && now.Before(state.rotateUntil)
	frameStatesMu.Lock()
	if resetNavSwitch {
		state.navSwitchUntil = time.Time{}
	}
	frameStates[key] = &frameMotionState{
		scaleReflow:          scaleReflow,
		orientationRotating:  orientationRotating,
		software:             software,
		orientationRotateGen: state.rotateGen,
		layout:               state,
	}
	frameStatesMu.Unlock()
	if scaleReflow {
		ctx.DisableAnimations()
		if now.Before(state.until) {
			ctx.After(state.until.Sub(now))
		}
	}
	return func() {
		frameStatesMu.Lock()
		delete(frameStates, key)
		frameStatesMu.Unlock()
	}
}

// SideContentMotion animates content shifts beside a toggling navigation rail.
func SideContentMotion(ctx *ui.Context, el ui.Element) ui.Element {
	if IsSoftware(ctx) {
		return el.Transition(sideContentTransitionCPU)
	}
	return el.Transition(SideContentTransition)
}

// NavigationPageMotion attaches a directional entrance transition for page switches.
func NavigationPageMotion(ctx *ui.Context, el ui.Element, horizontal bool, direction int) ui.Element {
	if IsSoftware(ctx) {
		return el
	}
	if direction == 0 {
		direction = 1
	}
	enter := &ui.Motion{Opacity: 0.45}
	if horizontal {
		enter.X = float32(direction * 14)
	} else {
		enter.Y = float32(direction * 10)
	}
	return el.Transition(ui.ElementTransition{
		Position: true, Animation: &motionPage, Enter: enter,
	})
}

// NavigationPageVeil renders a lightweight flat canvas fade veil over newly switched
// pages in software mode without offscreen groupLayer allocation or resampling.
func NavigationPageVeil(ctx *ui.Context, p Palette, page int) {
	if ctx == nil {
		return
	}
	reduceMotion := ctx.Preferences().ReduceMotion
	key := ctx.Services()
	now := ctx.Now()
	frameStatesMu.Lock()
	st := frameStates[key]
	if st == nil || st.staticSnapshot || st.layout == nil {
		frameStatesMu.Unlock()
		return
	}
	ls := st.layout
	if !ls.pageVeilInit {
		ls.pageVeilInit, ls.pageVeilPage = true, page
		frameStatesMu.Unlock()
		return
	}
	if !st.software {
		ls.pageVeilPage = page
		ls.pageVeilStart = time.Time{}
		frameStatesMu.Unlock()
		return
	}
	if st.scaleReflow || st.orientationRotating || reduceMotion {
		ls.pageVeilPage = page
		ls.pageVeilStart = time.Time{}
		frameStatesMu.Unlock()
		return
	}
	if ls.pageVeilPage != page {
		ls.pageVeilPage = page
		ls.pageVeilStart = now
	}
	start := ls.pageVeilStart
	elapsed := now.Sub(start)
	if start.IsZero() || elapsed < 0 || elapsed >= pageVeilDur {
		if !start.IsZero() && elapsed >= pageVeilDur {
			ls.pageVeilStart = time.Time{}
		}
		frameStatesMu.Unlock()
		return
	}
	frameStatesMu.Unlock()

	ctx.After(pageVeilDur - elapsed)
	canvasColor := p.Canvas
	ui.Box(ctx.Key("page-veil")).Absolute().Fill().PassThrough().IgnoreInput(true).AccessHidden(true).
		Draw(func(painter *ui.Painter, box ui.Rect) {
			dt := painter.Now().Sub(start)
			if dt < 0 || dt >= pageVeilDur {
				return
			}
			u := float32(dt) / float32(pageVeilDur)
			inv := 1 - u
			alpha := float32(0.55) * inv * inv * inv
			painter.Fill(box, canvasColor.Alpha(alpha), 0)
			painter.AnimationFrame()
		})
}

// AnimatedLabel renders a text element that morphs smoothly across locale changes.
func AnimatedLabel(ctx *ui.Context, text string) ui.Element {
	return ui.Text(ctx, text).ContentTransition(localeTextTransition)
}

// ControlMotion attaches layout transitions to text-bearing interactive controls.
func ControlMotion(el ui.Element) ui.Element {
	// Avoid Pose{Scale} on text-bearing controls so mygo glyph rasterization does not re-bucket fonts mid-press.
	return el.Transition(layoutTransition)
}

// CardMotion applies a subtle hover lift animation to interactive cards.
func CardMotion(el ui.Element) ui.Element {
	pose := ui.Pose{}
	if el.Hovered() && !el.IsDisabled() && !el.Pressed() {
		pose.Y = -1
	}
	return el.TransformOrigin(0.5, 0.5).AnimatePose(pose, motionSpring)
}

// SectionMotion attaches enter, exit, and sibling reflow transitions to dynamic
// vertical sections or cards (such as conditional FormGroup panels). Callers
// should key conditional elements with ctx.Key so sibling shifts stay anchored.
func SectionMotion(ctx *ui.Context, el ui.Element) ui.Element {
	if isStaticSnapshot(ctx) || isScaleReflow(ctx) {
		return el
	}
	return el.Transition(sectionTransition)
}

// InlineMotion attaches compact enter, exit, and reflow transitions to dynamic
// inline badges, status chips, subtitles, or conditional toolbar buttons.
func InlineMotion(ctx *ui.Context, el ui.Element) ui.Element {
	if isStaticSnapshot(ctx) || isScaleReflow(ctx) {
		return el
	}
	return el.Transition(inlineTransition)
}

func popupMotion(ctx *ui.Context, el ui.Element) {
	if IsSoftware(ctx) {
		el.AttachedOrigin().Transition(popupTransitionCPU)
		return
	}
	el.AttachedOrigin().Transition(popupTransition)
}

func optionMenuMotion(ctx *ui.Context, el ui.Element) ui.Element {
	if IsSoftware(ctx) {
		return el.AttachedOrigin().Transition(optionPopupTransitionCPU)
	}
	return el.AttachedOrigin().Transition(optionPopupTransition)
}

func modalMotion(ctx *ui.Context, el ui.Element) ui.Element {
	if IsSoftware(ctx) {
		return el.TransformOrigin(0.5, 0.5).Transition(modalTransitionCPU)
	}
	return el.TransformOrigin(0.5, 0.5).Transition(modalTransition)
}

func sheetMotion(ctx *ui.Context, el ui.Element) ui.Element {
	if IsSoftware(ctx) {
		return el.TransformOrigin(0.5, 1).Transition(sheetTransitionCPU)
	}
	return el.TransformOrigin(0.5, 1).Transition(sheetTransition)
}

func backdropMotion(ctx *ui.Context, el ui.Element, p Palette) ui.Element {
	bg := p.Backdrop
	if p.LiquidGlass {
		bg = ui.RGBA(8, 10, 14, .24)
	}
	el = el.Background(bg)
	if IsSoftware(ctx) {
		return el
	}
	return el.Transition(backdropTransition)
}
