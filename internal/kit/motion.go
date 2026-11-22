package kit

import (
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	MotionFastDur        = 120 * time.Millisecond
	MotionNavDur         = 175 * time.Millisecond
	MotionMoveDur        = 220 * time.Millisecond
	MotionPopupOpenDur   = 180 * time.Millisecond
	MotionPopupCloseDur  = 100 * time.Millisecond
	MotionOptionDur      = 220 * time.Millisecond
	MotionOptionCloseDur = 125 * time.Millisecond
	MotionPopupDur       = 230 * time.Millisecond
	MotionSheetDur       = 260 * time.Millisecond
	MotionPageDur        = 180 * time.Millisecond
	MotionUnfoldDur      = 280 * time.Millisecond
	rotateWindowDur      = 380 * time.Millisecond
)

var (
	EaseStandard   = ui.CubicBezier(0.2, 0, 0, 1)
	EaseAccelerate = ui.CubicBezier(0.4, 0, 1, 1)

	MotionFast         = ui.Animation{Duration: MotionFastDur, Ease: ui.EaseOut}
	MotionNav          = ui.Animation{Duration: MotionNavDur, Ease: EaseStandard}
	MotionChipOut      = ui.Animation{Duration: time.Nanosecond}
	MotionChipIn       = ui.Animation{Delay: MotionOptionCloseDur, Duration: 75 * time.Millisecond, Ease: EaseStandard}
	MotionCapsuleIn    = ui.Animation{Duration: 120 * time.Millisecond, Ease: ui.EaseOut}
	MotionCapsuleDrag  = ui.Animation{Duration: 22 * time.Millisecond, Ease: ui.EaseOut}
	MotionCapsulePill  = ui.Animation{Duration: MotionNavDur, Ease: EaseStandard}
	MotionMove         = ui.Animation{Duration: MotionMoveDur, Ease: ui.EaseOut}
	MotionLocale       = ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.04}}
	MotionPage         = ui.Animation{Duration: MotionPageDur, Ease: EaseStandard}
	MotionRail         = ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.14}}
	MotionGlassPress   = ui.Animation{Spring: ui.SpringOptions{Duration: 115 * time.Millisecond, Bounce: 0.24}}
	MotionGlassRelease = ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond, Bounce: 0.28}}
	MotionGlassScrub   = ui.Animation{Spring: ui.SpringOptions{Duration: 180 * time.Millisecond, Bounce: 0.22}}
	MotionNavPanel     = ui.Animation{Spring: ui.SpringOptions{Duration: 340 * time.Millisecond, Bounce: 0.14}}
	MotionRotate       = ui.Animation{Spring: ui.SpringOptions{Duration: 320 * time.Millisecond, Bounce: 0.04}}
	MotionRotatePanel  = ui.Animation{Spring: ui.SpringOptions{Duration: 290 * time.Millisecond, Bounce: 0.05}}
	MotionSpring       = ui.Animation{Spring: ui.SpringOptions{Duration: MotionMoveDur, Bounce: 0.06}}
	MotionOptionSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 230 * time.Millisecond, Bounce: 0.06}}
	MotionPopup        = ui.Animation{Duration: MotionPopupOpenDur, Ease: EaseStandard}
	MotionOption       = ui.Animation{Duration: MotionOptionDur, Ease: EaseStandard}
	MotionOptionClose  = ui.Animation{Duration: MotionOptionCloseDur, Ease: EaseStandard}
	MotionDialogPop    = ui.Animation{Spring: ui.SpringOptions{Duration: MotionPopupDur, Bounce: 0.06}}
	MotionSheet        = ui.Animation{Spring: ui.SpringOptions{Duration: MotionSheetDur, Bounce: 0.04}}

	ValueMotion          = ui.ContentTransition{Animation: &MotionMove}
	LocaleTextTransition = ui.ContentTransition{Morph: true, Animation: &MotionLocale}
	LocaleSizeTransition = ui.ElementTransition{Position: true, Size: true, Animation: &MotionLocale}

	LayoutTransition      = ui.ElementTransition{Position: true, Size: true, Animation: &MotionMove}
	SideContentTransition = ui.ElementTransition{
		Position:  true,
		Size:      true,
		Animation: &MotionNavPanel,
	}
	PopupTransition = ui.ElementTransition{
		Duration:     MotionPopupOpenDur,
		Ease:         EaseStandard,
		ExitDuration: MotionPopupCloseDur,
		ExitEase:     EaseAccelerate,
		Position:     true,
		Enter:        &ui.Motion{Y: -2, Scale: 0.88, Flip: true},
		Exit:         &ui.Motion{Y: -1, Scale: 0.92, Flip: true},
	}
	OptionPopupTransition = ui.ElementTransition{
		Animation:    &MotionOptionSpring,
		ExitDuration: MotionOptionCloseDur,
		ExitEase:     EaseStandard,
		Enter:        &ui.Motion{Scale: 0.78, Opacity: 1},
		Exit:         &ui.Motion{Scale: 0.88, Opacity: 0},
	}
	ModalTransition = ui.ElementTransition{
		Position: true, Colors: true, Animation: &MotionDialogPop,
		Enter: &ui.Motion{Y: 10, Scale: 0.94},
		Exit:  &ui.Motion{Y: 6, Scale: 0.96},
	}
	SheetTransition = ui.ElementTransition{
		Position: true, Colors: true, Animation: &MotionSheet,
		Enter: &ui.Motion{Y: 56},
		Exit:  &ui.Motion{Y: 40},
	}
	BackdropTransition = ui.ElementTransition{
		Animation: &MotionMove,
		Colors:    true,
		Enter:     &ui.Motion{},
		Exit:      &ui.Motion{},
	}
)

type frameMotionState struct {
	scaleReflow          bool
	staticSnapshot       bool
	orientationRotating  bool
	orientationRotateGen uint64
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

// IsStaticSnapshot reports whether ctx is painting a static reveal snapshot.
func IsStaticSnapshot(ctx *ui.Context) bool {
	return readFrameMotionState(ctx).staticSnapshot
}

// IsScaleReflow reports whether ctx is suppressing layout transitions after a resize or inset change.
func IsScaleReflow(ctx *ui.Context) bool {
	return readFrameMotionState(ctx).scaleReflow
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

// GlassPressAnimation returns the spring for interactive Liquid Glass press or release.
func GlassPressAnimation(pressed bool) ui.Animation {
	if pressed {
		return MotionGlassPress
	}
	return MotionGlassRelease
}

func PillMatchTransition(ctx *ui.Context, matchKey any) ui.ElementTransition {
	return pillMatchTransition(ctx, matchKey, &MotionRail)
}

func pillMatchTransition(ctx *ui.Context, matchKey any, anim *ui.Animation) ui.ElementTransition {
	if IsStaticSnapshot(ctx) {
		return ui.ElementTransition{}
	}
	return ui.ElementTransition{
		Position: true, Size: true, Colors: true,
		Animation: anim, Match: matchKey,
	}
}

// Desktop navigation shares one spring so its tabs and toggle stay with the panel.
func navMatchTransition(ctx *ui.Context, matchKey any, shadows bool) ui.ElementTransition {
	if IsStaticSnapshot(ctx) {
		return ui.ElementTransition{}
	}
	return ui.ElementTransition{
		Position:  true,
		Size:      true,
		Colors:    true,
		Shadows:   shadows,
		Animation: &MotionNavPanel,
		Match:     matchKey,
		Fly:       true,
	}
}

// NavRotateMatchTransition returns a matched-geometry transition for mobile
// elements whose layout moves across portrait <-> landscape rotations while
// snapping inset or same-orientation updates immediately.
func NavRotateMatchTransition(ctx *ui.Context, matchKey any, shadows bool) ui.ElementTransition {
	fs := readFrameMotionState(ctx)
	if fs.staticSnapshot {
		return ui.ElementTransition{}
	}
	return ui.ElementTransition{
		Position:  fs.orientationRotating,
		Size:      fs.orientationRotating,
		Colors:    true,
		Shadows:   shadows,
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
	// Hold the start pose through any multi-pass rebuild or same-timestamp inset settle on frame 0, then release 1ms later.
	if st.gen != 0 && !now.After(st.startedAt) {
		pose = start
		ctx.After(time.Millisecond)
	}
	return el.AnimatePose(pose, anim)
}

func LayoutAnimation(ctx *ui.Context, animation ui.Animation) ui.Animation {
	if IsScaleReflow(ctx) {
		return ui.Animation{Duration: time.Nanosecond}
	}
	return animation
}

// BeginFrame prepares the overlay tree, enables matched orientation transitions
// when a mobile viewport rotates, and suppresses positional transitions on
// desktop window resizes or same-orientation safe-area inset updates.
func BeginFrame(ctx *ui.Context, mobile bool) func() {
	ui.Overlay(ctx, func() {})
	type frameState struct {
		initialized              bool
		w, h                     float32
		top, right, bottom, left float32
		until                    time.Time
		rotateUntil              time.Time
		rotateGen                uint64
	}
	w, h := ctx.Size()
	top, right, bottom, left := ctx.SafeArea()
	state := ui.Local(ui.Box(ctx).Absolute().Size(0, 0).PassThrough(), "frame-layout", func() frameState {
		return frameState{}
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
				state.until = now.Add(MotionUnfoldDur)
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

func NavigationPageMotion(el ui.Element, horizontal bool, direction int) ui.Element {
	if direction == 0 {
		direction = 1
	}
	enter := &ui.Motion{Opacity: 0.72}
	if horizontal {
		enter.X = float32(direction * 12)
	} else {
		enter.Y = float32(direction * 10)
	}
	return el.Transition(ui.ElementTransition{
		Position: true, Animation: &MotionPage, Enter: enter,
	})
}

func AnimatedLabel(ctx *ui.Context, text string) ui.Element {
	return ui.Text(ctx, text).ContentTransition(LocaleTextTransition)
}

func ControlMotion(el ui.Element) ui.Element {
	// Avoid Pose{Scale} on text-bearing controls so mygo glyph rasterization does not re-bucket fonts mid-press.
	return el.Transition(LayoutTransition)
}

func CardMotion(el ui.Element) ui.Element {
	pose := ui.Pose{}
	if el.Hovered() && !el.IsDisabled() && !el.Pressed() {
		pose.Y = -1
	}
	return el.TransformOrigin(0.5, 0.5).AnimatePose(pose, MotionSpring)
}

func PopupMotion(el ui.Element) ui.Element {
	return el.AttachedOrigin().Transition(PopupTransition)
}

func PopupMotionAt(el ui.Element, originX, originY float32) ui.Element {
	return el.TransformOrigin(originX, originY).Transition(PopupTransition)
}

func OptionMenuMotion(el ui.Element) ui.Element {
	return el.AttachedOrigin().Transition(OptionPopupTransition)
}

func ModalMotion(el ui.Element) ui.Element {
	return el.TransformOrigin(0.5, 0.5).Transition(ModalTransition)
}

func SheetMotion(el ui.Element) ui.Element {
	return el.TransformOrigin(0.5, 1).Transition(SheetTransition)
}

func BackdropMotion(el ui.Element, p Palette) ui.Element {
	bg := p.Backdrop
	if p.LiquidGlass {
		bg = ui.RGBA(8, 10, 14, .24)
	}
	return el.Background(bg).Transition(BackdropTransition)
}

func ScrolledEdge(el ui.Element, p Palette) ui.Element {
	color := p.LineSoft
	return el.DrawOver(func(painter *ui.Painter, rect ui.Rect) {
		painter.Fill(ui.Rect{X: rect.X, Y: rect.Y, W: rect.W, H: 1}, color, 0)
	})
}
