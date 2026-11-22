package kit

import (
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	motionFastDur        = 120 * time.Millisecond
	motionNavDur         = 175 * time.Millisecond
	motionMoveDur        = 220 * time.Millisecond
	motionPopupOpenDur   = 180 * time.Millisecond
	motionPopupCloseDur  = 110 * time.Millisecond
	motionOptionCloseDur = 125 * time.Millisecond
	motionPopupDur       = 230 * time.Millisecond
	motionSheetDur       = 260 * time.Millisecond
	motionPageDur        = 235 * time.Millisecond
	motionUnfoldDur      = 280 * time.Millisecond
	rotateWindowDur      = 380 * time.Millisecond
	navLayoutSwitchDur   = 360 * time.Millisecond
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
	// MotionRotate is the spring animation used when elements transition across mobile orientations.
	MotionRotate       = ui.Animation{Spring: ui.SpringOptions{Duration: 320 * time.Millisecond, Bounce: 0.04}}
	motionRotatePanel  = ui.Animation{Spring: ui.SpringOptions{Duration: 290 * time.Millisecond, Bounce: 0.05}}
	motionSpring       = ui.Animation{Spring: ui.SpringOptions{Duration: motionMoveDur, Bounce: 0.06}}
	motionOptionSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 230 * time.Millisecond, Bounce: 0.06}}
	motionPopup        = ui.Animation{Duration: motionPopupOpenDur, Ease: easeStandard}
	motionPopupSpring  = ui.Animation{Spring: ui.SpringOptions{Duration: 220 * time.Millisecond, Bounce: 0.09}}
	motionDialogPop    = ui.Animation{Spring: ui.SpringOptions{Duration: motionPopupDur, Bounce: 0.06}}
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
	popupTransition = ui.ElementTransition{
		Animation:    &motionPopupSpring,
		ExitDuration: motionPopupCloseDur,
		ExitEase:     easeAccelerate,
		Position:     true,
		Enter:        &ui.Motion{Y: -3, Scale: 0.90, Flip: true},
		Exit:         &ui.Motion{Y: -1, Scale: 0.94, Flip: true},
	}
	optionPopupTransition = ui.ElementTransition{
		Animation:    &motionOptionSpring,
		ExitDuration: motionOptionCloseDur,
		ExitEase:     easeStandard,
		Enter:        &ui.Motion{Scale: 0.78, Opacity: 1},
		Exit:         &ui.Motion{Scale: 0.88, Opacity: 0},
	}
	modalTransition = ui.ElementTransition{
		Position: true, Colors: true, Animation: &motionDialogPop,
		Enter: &ui.Motion{Y: 10, Scale: 0.94},
		Exit:  &ui.Motion{Y: 6, Scale: 0.96},
	}
	sheetTransition = ui.ElementTransition{
		Position: true, Colors: true, Animation: &motionSheet,
		Enter: &ui.Motion{Y: 56},
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
}

type frameMotionState struct {
	scaleReflow          bool
	staticSnapshot       bool
	orientationRotating  bool
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

func trackDesktopNavLayoutSwitch(ctx *ui.Context, sidebar bool) bool {
	if ctx == nil || isStaticSnapshot(ctx) {
		return false
	}
	frameStatesMu.Lock()
	st := frameStates[ctx.Services()]
	if st == nil || st.layout == nil {
		frameStatesMu.Unlock()
		return false
	}
	ls := st.layout
	if !ls.navSidebarInit {
		ls.navSidebarInit, ls.navSidebar = true, sidebar
		frameStatesMu.Unlock()
		return false
	}
	now := ctx.Now()
	if ls.navSidebar != sidebar {
		ls.navSidebar = sidebar
		ls.navSwitchUntil = now.Add(navLayoutSwitchDur)
		frameStatesMu.Unlock()
		ctx.After(navLayoutSwitchDur)
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
	if isStaticSnapshot(ctx) {
		return ui.ElementTransition{}
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
	state := ui.Local(ui.Box(ctx).Absolute().Size(0, 0).PassThrough(), "frame-layout", func() frameLayoutState {
		return frameLayoutState{}
	})
	now := ctx.Now()
	insetSnap := false
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
			case sameSideSettling:
				// Preserve ongoing rotation springs if Android finishes bar/cutout sizing a frame later.
			default:
				state.until = now.Add(motionUnfoldDur)
				state.rotateUntil = time.Time{}
			}
		case mobile && (state.top != top || state.right != right || state.bottom != bottom || state.left != left):
			state.top, state.right, state.bottom, state.left = top, right, bottom, left
			if !now.Before(state.rotateUntil) {
				state.rotateUntil = time.Time{}
				insetSnap = true
			}
		}
	}
	scaleReflow := insetSnap || now.Before(state.until)
	orientationRotating := mobile && now.Before(state.rotateUntil)
	key := ctx.Services()
	frameStatesMu.Lock()
	frameStates[key] = &frameMotionState{
		scaleReflow:          scaleReflow,
		orientationRotating:  orientationRotating,
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

// NavigationPageMotion attaches a directional entrance transition for page switches.
func NavigationPageMotion(el ui.Element, horizontal bool, direction int) ui.Element {
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

func popupMotion(el ui.Element) {
	el.AttachedOrigin().Transition(popupTransition)
}

func optionMenuMotion(el ui.Element) ui.Element {
	return el.AttachedOrigin().Transition(optionPopupTransition)
}

func modalMotion(el ui.Element) ui.Element {
	return el.TransformOrigin(0.5, 0.5).Transition(modalTransition)
}

func sheetMotion(el ui.Element) ui.Element {
	return el.TransformOrigin(0.5, 1).Transition(sheetTransition)
}

func backdropMotion(el ui.Element, p Palette) ui.Element {
	bg := p.Backdrop
	if p.LiquidGlass {
		bg = ui.RGBA(8, 10, 14, .24)
	}
	return el.Background(bg).Transition(backdropTransition)
}
