package kit

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

type navScrubAxis uint8

const (
	navAxisHorizontal navScrubAxis = iota
	navAxisVertical
)

type navSlot struct {
	start  float32
	extent float32
}

const (
	navScrubDragThresh      float32 = 12
	navScrubGlassDragThresh float32 = 6
	navScrubHoldDelay               = 180 * time.Millisecond
)

type navScrubState struct {
	pressing      bool
	active        bool
	upPending     bool
	canceled      bool
	suppressClick bool
	pressedAt     time.Time
	originMain    float32
	lastX         float32
	lastY         float32
	dragIndex     int
	prevMain      float32
	lastTime      time.Time
	hasPrev       bool
	lastMode      uint8
	lastCount     int
}

type navScrubInput struct {
	now         time.Time
	x, y        float32
	mainPos     float32
	anyPressed  bool
	anyCanceled bool
	released    bool
	dragThresh  float32
	holdDelay   time.Duration
	selected    int
	slots       []navSlot
	gap         float32
	allowArm    bool
}

type navScrubOutcome struct {
	activeChanged bool
	commit        bool
	commitIndex   int
	waitForHold   time.Duration
}

type navLensStyle struct {
	activePos  ui.SpringOptions
	idlePos    ui.SpringOptions
	extSpring  ui.SpringOptions
	liftOn     ui.SpringOptions
	liftOff    ui.SpringOptions
	stretchDiv float32
	stretchMax float32
	liftFlight float32
}

var (
	navLensDesktopStyle = navLensStyle{
		activePos: ui.SpringOptions{Duration: 45 * time.Millisecond, Bounce: 0.04},
		idlePos:   MotionGlassScrub.Spring, extSpring: MotionGlassScrub.Spring,
		liftOn: GlassPressAnimation(true).Spring, liftOff: GlassPressAnimation(false).Spring,
		stretchDiv: 1100, stretchMax: 0.20, liftFlight: 0.78,
	}
	navLensBottomCapsuleStyle = navLensStyle{
		activePos: ui.SpringOptions{Duration: 155 * time.Millisecond, Bounce: 0.16},
		idlePos:   MotionGlassScrub.Spring, extSpring: MotionGlassScrub.Spring,
		liftOn: GlassPressAnimation(true).Spring, liftOff: GlassPressAnimation(false).Spring,
		stretchDiv: 750, stretchMax: 0.20, liftFlight: 0.76,
	}
	navLensBottomStandardStyle = navLensStyle{
		activePos: ui.SpringOptions{Duration: 40 * time.Millisecond},
		idlePos:   ui.SpringOptions{Duration: 190 * time.Millisecond, Bounce: 0.14}, extSpring: MotionGlassScrub.Spring,
		liftOn: ui.SpringOptions{Duration: 200 * time.Millisecond, Bounce: 0.16}, liftOff: ui.SpringOptions{Duration: 290 * time.Millisecond, Bounce: 0.18},
		stretchDiv: 1400, stretchMax: 0.10, liftFlight: 0.76,
	}
)

type navLensConfig struct {
	axis       navScrubAxis
	target     ui.Rect
	mode       uint8
	count      int
	keyPrefix  string
	style      navLensStyle
	tabPressed bool
	restHidden bool
	forceJump  bool
	expand     func(mainExt, crossExt, elastic, stretch float32) (dx, dy, padCross float32)
}

type navGlassLens struct {
	base             ui.Rect
	bounds           ui.Rect
	touch            ui.Point
	padCross         float32
	alpha, glow      float32
	inFlight, optics bool
}

func (l navGlassLens) box(ctx *ui.Context, radius float32, z int) ui.Element {
	return ui.Box(ctx).Absolute().Left(l.bounds.X).Top(l.bounds.Y).
		Width(l.bounds.W).Height(l.bounds.H).ZIndex(z).Radius(radius).PassThrough()
}

func navMainCoord(axis navScrubAxis, x, y float32) float32 {
	if axis == navAxisVertical {
		return y
	}
	return x
}

func navScrubDragThreshold(liquidGlass bool) float32 {
	if liquidGlass {
		return navScrubGlassDragThresh
	}
	return navScrubDragThresh
}

func navSlotIndexAt(mainPos, gap float32, slots []navSlot) int {
	if len(slots) == 0 {
		return 0
	}
	for i := 0; i+1 < len(slots); i++ {
		curEnd := slots[i].start + slots[i].extent
		nextStart := slots[i+1].start
		boundary := curEnd + gap*0.5
		if nextStart > curEnd {
			boundary = (curEnd + nextStart) * 0.5
		}
		if mainPos < boundary {
			return i
		}
	}
	return len(slots) - 1
}

func navSlotContains(mainPos, gap float32, slots []navSlot) (int, bool) {
	halfGap := max(0, gap) * 0.5
	for i, slot := range slots {
		if slot.extent > 0 && mainPos >= slot.start-halfGap && mainPos <= slot.start+slot.extent+halfGap {
			return i, true
		}
	}
	return -1, false
}

func stepNavScrub(st *navScrubState, in navScrubInput) navScrubOutcome {
	var out navScrubOutcome
	if !in.released && !in.anyPressed {
		st.suppressClick = false
	}
	reset := func() {
		if st.active {
			out.activeChanged = true
		}
		st.pressing, st.active, st.upPending, st.canceled = false, false, false, false
		st.pressedAt = time.Time{}
	}
	switch {
	case in.released:
		if st.pressing && st.active && !in.anyCanceled && len(in.slots) > 0 {
			out.commit = true
			out.commitIndex = clamp32Index(st.dragIndex, len(in.slots))
			st.suppressClick = true
		}
		reset()
	case in.anyCanceled || (st.pressing && !in.anyPressed):
		reset()
	case in.anyPressed:
		if !st.pressing {
			if !in.allowArm {
				return out
			}
			st.pressing = true
			st.pressedAt = in.now
			st.originMain = in.mainPos
			st.dragIndex = clamp32Index(in.selected, len(in.slots))
		} else if st.pressedAt.IsZero() {
			st.pressedAt = in.now
		}
		st.lastX = in.x
		st.lastY = in.y
		if !st.active {
			moved := abs32(in.mainPos-st.originMain) >= in.dragThresh
			held := in.holdDelay > 0 && !st.pressedAt.IsZero() && in.now.Sub(st.pressedAt) >= in.holdDelay
			if moved || held {
				st.active = true
				out.activeChanged = true
			} else if in.holdDelay > 0 && !st.pressedAt.IsZero() {
				if rem := in.holdDelay - in.now.Sub(st.pressedAt); rem > 0 {
					out.waitForHold = rem
				}
			}
		}
		if st.active && len(in.slots) > 0 {
			st.dragIndex = navSlotIndexAt(in.mainPos, in.gap, in.slots)
		}
	}
	return out
}

func (st *navScrubState) stepTabs(ctx *ui.Context, host ui.Element, tabEls []ui.Element, axis navScrubAxis, gap float32, slots []navSlot, liquidGlass, requireTabArm bool, selected *int, items []NavTab) (displayIndex int, anyPressed bool) {
	hostPressed := host.Pressed()
	if b, ok := host.PressedButton(); ok && b == 0 {
		hostPressed = true
	}
	_, _, hostDragHeld := host.Dragged()
	_, _, hostDragEnded := host.DragEnded()
	hostCanceled := host.PressCanceled()
	px, py, _ := host.PointerPosition()

	var tabPressed, tabCanceled, tabDragEnded bool
	for _, tab := range tabEls {
		if b, ok := tab.PressedButton(); tab.Pressed() || (ok && b == 0) {
			tabPressed = true
		}
		if _, _, ended := tab.DragEnded(); ended {
			tabDragEnded = true
		}
		if tab.PressCanceled() {
			tabCanceled = true
		}
	}

	anyPressed = hostPressed || hostDragHeld || tabPressed
	anyCanceled := st.canceled || hostCanceled || tabCanceled
	dragEnded := hostDragEnded || tabDragEnded
	// A tab can emit a synthetic release when the host takes over the drag.
	if anyPressed && !anyCanceled && !dragEnded {
		st.upPending = false
	}

	outcome := stepNavScrub(st, navScrubInput{
		now:         ctx.Now(),
		x:           px,
		y:           py,
		mainPos:     navMainCoord(axis, px, py),
		anyPressed:  anyPressed,
		anyCanceled: anyCanceled,
		released:    st.upPending || dragEnded,
		dragThresh:  navScrubDragThreshold(liquidGlass),
		holdDelay:   navScrubHoldDelay,
		selected:    *selected,
		slots:       slots,
		gap:         gap,
		allowArm:    !requireTabArm || tabPressed,
	})
	if outcome.waitForHold > 0 {
		ctx.After(outcome.waitForHold)
	}
	if outcome.commit {
		target := clamp32Index(outcome.commitIndex, len(items))
		*selected = target
		if items[target].OnSelect != nil {
			items[target].OnSelect()
		}
	}
	displayIndex = clamp32Index(*selected, len(items))
	if st.active {
		displayIndex = clamp32Index(st.dragIndex, len(items))
	}
	return displayIndex, anyPressed
}

func navTabClick(st *navScrubState, onSelect func()) func() {
	return func() {
		if st.suppressClick {
			st.suppressClick = false
			return
		}
		if onSelect != nil {
			onSelect()
		}
	}
}

func (s *navScrubState) animateGlassLens(ctx *ui.Context, host ui.Element, cfg navLensConfig) navGlassLens {
	now := ctx.Now()
	curMain, curExt := cfg.target.X, cfg.target.W
	crossOff, crossExt := cfg.target.Y, cfg.target.H
	if cfg.axis == navAxisVertical {
		curMain, curExt = cfg.target.Y, cfg.target.H
		crossOff, crossExt = cfg.target.X, cfg.target.W
	}
	targetMain, targetExt := curMain, curExt

	var stretch, lift float32
	var inFlight bool
	if IsStaticSnapshot(ctx) || IsScaleReflow(ctx) || cfg.forceJump || !s.hasPrev || s.lastMode != cfg.mode || s.lastCount != cfg.count {
		host.Jump(cfg.keyPrefix+"-main", curMain)
		host.Jump(cfg.keyPrefix+"-ext", curExt)
		host.Jump(cfg.keyPrefix+"-stretch", 0)
		host.Jump(cfg.keyPrefix+"-lift", 0)
		s.hasPrev, s.lastMode, s.lastCount = true, cfg.mode, cfg.count
	} else {
		posSpring := cfg.style.idlePos
		if s.active {
			posSpring = cfg.style.activePos
		}
		curMain = host.Spring(cfg.keyPrefix+"-main", curMain, posSpring)
		curExt = host.Spring(cfg.keyPrefix+"-ext", curExt, cfg.style.extSpring)
		var stretchTarget float32
		if dt := now.Sub(s.lastTime).Seconds(); dt > 0.0005 && dt < 0.25 {
			speed := abs32(curMain-s.prevMain) / float32(dt)
			stretchTarget = clamp32(speed/cfg.style.stretchDiv, 0, cfg.style.stretchMax)
		}
		stretch = clamp32(host.Spring(cfg.keyPrefix+"-stretch", stretchTarget, cfg.style.extSpring), 0, 0.24)
		inFlight = abs32(curMain-targetMain) > 3 || abs32(curExt-targetExt) > 3
		var liftTarget float32
		switch {
		case s.active || (!cfg.restHidden && cfg.tabPressed):
			liftTarget = 1
		case inFlight:
			liftTarget = cfg.style.liftFlight
		}
		liftSpring := cfg.style.liftOff
		if liftTarget > 0.5 {
			liftSpring = cfg.style.liftOn
		}
		lift = host.Spring(cfg.keyPrefix+"-lift", liftTarget, liftSpring)
	}
	s.prevMain, s.lastTime = curMain, now
	alpha := clamp32(lift, 0, 1)
	elastic := clamp32(lift, -0.18, 1.28)
	dx, dy, padCross := cfg.expand(curExt, crossExt, elastic, stretch)
	base := ui.Rect{X: curMain, Y: crossOff, W: curExt, H: crossExt}
	if cfg.axis == navAxisVertical {
		base = ui.Rect{X: crossOff, Y: curMain, W: crossExt, H: curExt}
	}
	bounds := expandGlass(base, dx, dy)
	touch := ui.Point{X: 0.5, Y: 0.5}
	optics := alpha > 0.005 && !ctx.Preferences().ReduceTransparency
	if optics && s.pressing {
		touch.X = clamp32((s.lastX-bounds.X)/bounds.W, 0.08, 0.92)
		touch.Y = clamp32((s.lastY-bounds.Y)/bounds.H, 0.12, 0.88)
	}
	return navGlassLens{
		base:     base,
		bounds:   bounds,
		touch:    touch,
		padCross: padCross,
		alpha:    alpha,
		glow:     max(alpha, stretch*1.25),
		inFlight: inFlight,
		optics:   optics,
	}
}

func navSolidScrubPose(axis navScrubAxis, active bool, lastMain, stripMain, maxNudge, rubberLimit float32) (ui.Pose, ui.Animation) {
	if !active || stripMain <= 0 {
		return ui.Pose{}, MotionNav
	}
	var overscroll float32
	switch {
	case lastMain < 0:
		overscroll = lastMain
	case lastMain > stripMain:
		overscroll = lastMain - stripMain
	}
	nudge := clamp32((lastMain-stripMain*0.5)*0.035, -maxNudge, maxNudge) + navScrubRubberBand(overscroll, rubberLimit)
	var pose ui.Pose
	if axis == navAxisVertical {
		pose.Y = nudge
	} else {
		pose.X = nudge
	}
	return pose, MotionCapsuleDrag
}

func navSolidPillFeedback(active, pressed bool, lastMain, stripMain float32, baseBg, textColor ui.Color, activeMix, pressMix float32) (bg ui.Color, pose ui.Pose, pillAnim, poseAnim ui.Animation) {
	bg = baseBg
	pillAnim = MotionCapsulePill
	poseAnim = MotionNav
	switch {
	case active:
		bg = baseBg.Mix(textColor, activeMix)
		pillAnim = MotionCapsuleDrag
		poseAnim = MotionCapsuleDrag
		if stripMain > 0 {
			align := clamp32((lastMain/stripMain)*2-1, -1, 1)
			edge := clamp32((abs32(align)-0.72)/0.28, 0, 1)
			pose.Scale = 1 + edge*0.025
		}
	case pressed:
		bg = baseBg.Mix(textColor, pressMix)
		pose.Scale = 0.96
		poseAnim = MotionFast
	}
	return bg, pose, pillAnim, poseAnim
}

func navLensShadow(el ui.Element, dark bool, amount float32, standard bool) {
	if standard {
		alpha := float32(0.068)
		if dark {
			alpha = 0.26
		}
		el.Shadow(0, 2.0*amount, 9.5*amount, 0, ui.RGBA(12, 18, 30, alpha*min(amount, 1)))
		return
	}
	alpha := float32(0.075)
	if dark {
		alpha = 0.24
	}
	el.Shadow(0, 1.5*amount, 8*amount, 0, ui.RGBA(12, 18, 30, alpha*min(amount, 1)))
}

func drawNavGlassLens(ctx *ui.Context, el ui.Element, radius float32, spec liquidLensSpec) {
	if IsStaticSnapshot(ctx) || spec.Alpha <= 0.005 {
		return
	}
	el.Draw(func(p *ui.Painter, box ui.Rect) {
		r := radius
		if r <= 0 {
			r = box.H * 0.5
		} else {
			r = min(r, min(box.W, box.H)*0.5)
		}
		paintLiquidGlass(p, box, r, spec)
	})
}

func navScrubRubberBand(overscroll, limit float32) float32 {
	if overscroll == 0 || limit <= 0 {
		return 0
	}
	return limit * float32(math.Tanh(float64(overscroll)/float64(limit*2.5)))
}

func (s *navScrubState) observePointer(axis navScrubAxis, gap float32, slots []navSlot, selected, count int, checkSlot bool) func(ui.InputEvent) {
	return func(ev ui.InputEvent) {
		if ev.Button != 0 {
			return
		}
		switch ev.Kind {
		case ui.InputPointerDown:
			mainPos := navMainCoord(axis, ev.X, ev.Y)
			if checkSlot && len(slots) > 0 {
				if _, ok := navSlotContains(mainPos, gap, slots); !ok {
					return
				}
			}
			s.pressing = true
			s.active = false
			s.upPending = false
			s.canceled = false
			s.suppressClick = false
			s.pressedAt = time.Time{}
			s.originMain = mainPos
			s.lastX = ev.X
			s.lastY = ev.Y
			s.dragIndex = clamp32Index(selected, count)
		case ui.InputPointerUp:
			if s.pressing {
				s.upPending = true
				if ev.Canceled {
					s.canceled = true
				}
			}
		}
	}
}
