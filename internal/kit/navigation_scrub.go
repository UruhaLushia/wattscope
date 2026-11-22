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
	hasMeasured   bool
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
	activePos     ui.SpringOptions
	idlePos       ui.SpringOptions
	extSpring     ui.SpringOptions
	stretchSpring ui.SpringOptions
	liftOn        ui.SpringOptions
	liftOff       ui.SpringOptions
	stretchDiv    float32
	stretchMax    float32
	liftFlight    float32
}

var (
	navLensDesktopStyle = navLensStyle{
		activePos:     ui.SpringOptions{Duration: 45 * time.Millisecond, Bounce: 0.04},
		idlePos:       ui.SpringOptions{Duration: 240 * time.Millisecond, Bounce: 0.12},
		extSpring:     ui.SpringOptions{Duration: 215 * time.Millisecond, Bounce: 0.12},
		stretchSpring: ui.SpringOptions{Duration: 115 * time.Millisecond, Bounce: 0.08},
		liftOn:        ui.SpringOptions{Duration: 165 * time.Millisecond, Bounce: 0.14},
		liftOff:       ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.12},
		stretchDiv:    950, stretchMax: 0.22, liftFlight: 0.88,
	}
	navLensBottomCapsuleStyle = navLensStyle{
		activePos:     ui.SpringOptions{Duration: 155 * time.Millisecond, Bounce: 0.16},
		idlePos:       motionGlassScrub.Spring,
		extSpring:     motionGlassScrub.Spring,
		stretchSpring: ui.SpringOptions{Duration: 110 * time.Millisecond, Bounce: 0.08},
		liftOn:        motionGlassPress.Spring, liftOff: motionGlassRelease.Spring,
		stretchDiv: 750, stretchMax: 0.20, liftFlight: 0.76,
	}
	navLensBottomStandardStyle = navLensStyle{
		activePos:     ui.SpringOptions{Duration: 40 * time.Millisecond},
		idlePos:       ui.SpringOptions{Duration: 190 * time.Millisecond, Bounce: 0.14},
		extSpring:     motionGlassScrub.Spring,
		stretchSpring: ui.SpringOptions{Duration: 110 * time.Millisecond, Bounce: 0.08},
		liftOn:        ui.SpringOptions{Duration: 200 * time.Millisecond, Bounce: 0.16}, liftOff: ui.SpringOptions{Duration: 290 * time.Millisecond, Bounce: 0.18},
		stretchDiv: 1400, stretchMax: 0.10, liftFlight: 0.76,
	}
	navLensCompactStyle = navLensStyle{
		activePos:     ui.SpringOptions{Duration: 40 * time.Millisecond, Bounce: 0.02},
		idlePos:       ui.SpringOptions{Duration: 220 * time.Millisecond, Bounce: 0.12},
		extSpring:     ui.SpringOptions{Duration: 200 * time.Millisecond, Bounce: 0.12},
		stretchSpring: ui.SpringOptions{Duration: 105 * time.Millisecond, Bounce: 0.08},
		liftOn:        ui.SpringOptions{Duration: 160 * time.Millisecond, Bounce: 0.14},
		liftOff:       ui.SpringOptions{Duration: 250 * time.Millisecond, Bounce: 0.12},
		stretchDiv:    900, stretchMax: 0.18, liftFlight: 0.84,
	}
)

type navSlotCache struct {
	inline [4]navSlot
	extra  []navSlot
}

func (c *navSlotCache) slotsFor(n int) []navSlot {
	if n <= len(c.inline) {
		return c.inline[:n]
	}
	if len(c.extra) != n {
		c.extra = make([]navSlot, n)
	}
	return c.extra
}

type navLensConfig struct {
	axis       navScrubAxis
	target     ui.Rect
	mode       uint8
	count      int
	keyPrefix  string
	style      navLensStyle
	tabPressed bool
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

func rectBox(ctx *ui.Context, r ui.Rect, radius float32, z int) ui.Element {
	return ui.Box(ctx).Absolute().Left(r.X).Top(r.Y).Width(r.W).Height(r.H).ZIndex(z).Radius(radius).PassThrough()
}

func (l navGlassLens) box(ctx *ui.Context, radius float32, z int) ui.Element {
	return rectBox(ctx, l.bounds, radius, z)
}

func (l navGlassLens) coverAt(axis navScrubAxis, center, falloff float32) (cover, dist float32) {
	mid, ext := l.bounds.X+l.bounds.W*0.5, l.bounds.W
	if axis == navAxisVertical {
		mid, ext = l.bounds.Y+l.bounds.H*0.5, l.bounds.H
	}
	half := max(ext*0.5, 1)
	dist = abs32(center - mid)
	u := clamp32(1-dist/(half*falloff), 0, 1)
	return u * u * (3 - 2*u), dist
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

func (st *navScrubState) applyNavScrub(ctx *ui.Context, host ui.Element, itemEls []ui.Element, axis navScrubAxis, gap float32, slots []navSlot, liquidGlass, requireItemArm bool, selected *int) (displayIndex int, anyPressed, committed bool, commitIndex int) {
	hostPressed := host.Pressed()
	if b, ok := host.PressedButton(); ok && b == 0 {
		hostPressed = true
	}
	_, _, hostDragHeld := host.Dragged()
	_, _, hostDragEnded := host.DragEnded()
	hostCanceled := host.PressCanceled()
	px, py, _ := host.PointerPosition()

	var itemPressed, itemCanceled, itemDragEnded bool
	for _, el := range itemEls {
		if b, ok := el.PressedButton(); el.Pressed() || (ok && b == 0) {
			itemPressed = true
		}
		if _, _, ended := el.DragEnded(); ended {
			itemDragEnded = true
		}
		if el.PressCanceled() {
			itemCanceled = true
		}
	}

	anyPressed = hostPressed || hostDragHeld || itemPressed
	anyCanceled := st.canceled || hostCanceled || itemCanceled
	dragEnded := hostDragEnded || itemDragEnded
	// An item can emit a synthetic release when the host takes over the drag.
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
		allowArm:    !requireItemArm || itemPressed,
	})
	if outcome.waitForHold > 0 {
		ctx.After(outcome.waitForHold)
	}
	if outcome.commit {
		commitIndex = clamp32Index(outcome.commitIndex, len(itemEls))
		*selected = commitIndex
		committed = true
	}
	displayIndex = clamp32Index(*selected, len(itemEls))
	if st.active {
		displayIndex = clamp32Index(st.dragIndex, len(itemEls))
	}
	return displayIndex, anyPressed, committed, commitIndex
}

func (st *navScrubState) stepTabs(ctx *ui.Context, host ui.Element, tabEls []ui.Element, axis navScrubAxis, gap float32, slots []navSlot, liquidGlass, requireTabArm bool, selected *int, items []NavTab) (displayIndex int, anyPressed bool) {
	displayIndex, anyPressed, committed, target := st.applyNavScrub(ctx, host, tabEls, axis, gap, slots, liquidGlass, requireTabArm, selected)
	if committed && items[target].OnSelect != nil {
		items[target].OnSelect()
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
	if isStaticSnapshot(ctx) || isScaleReflow(ctx) || cfg.forceJump || !s.hasPrev || s.lastMode != cfg.mode || s.lastCount != cfg.count {
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
		var speed, stretchTarget float32
		if dt := now.Sub(s.lastTime).Seconds(); dt > 0.0005 && dt < 0.25 {
			speed = abs32(curMain-s.prevMain) / float32(dt)
			stretchTarget = clamp32(speed/cfg.style.stretchDiv, 0, cfg.style.stretchMax)
		}
		stretchSpring := cfg.style.stretchSpring
		if stretchSpring.Duration <= 0 {
			stretchSpring = ui.SpringOptions{Duration: 115 * time.Millisecond, Bounce: 0.08}
		}
		stretch = clamp32(host.Spring(cfg.keyPrefix+"-stretch", stretchTarget, stretchSpring), 0, 0.24)
		flightDist := max(abs32(curMain-targetMain), abs32(curExt-targetExt))
		inFlight = flightDist > 0.75 || speed > 36
		var liftTarget float32
		switch {
		case s.active || cfg.tabPressed:
			liftTarget = 1
		case inFlight:
			flightFactor := clamp32(flightDist/10+speed/550, 0, 1)
			liftTarget = cfg.style.liftFlight * flightFactor
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
		return ui.Pose{}, motionNav
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
	return pose, motionCapsuleDrag
}

func navSolidPillFeedback(active, pressed bool, lastMain, stripMain float32, baseBg, textColor ui.Color, activeMix, pressMix float32) (bg ui.Color, pose ui.Pose, pillAnim, poseAnim ui.Animation) {
	bg = baseBg
	pillAnim = motionNav
	poseAnim = motionNav
	switch {
	case active:
		bg = baseBg.Mix(textColor, activeMix)
		pillAnim = motionCapsuleDrag
		poseAnim = motionCapsuleDrag
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

func renderNavSolidPill(ctx *ui.Context, host ui.Element, key any, rect ui.Rect, radius float32, colorKey string, bg ui.Color, pose ui.Pose, moveAnim, poseAnim ui.Animation, animateLayout bool, shadow bool, shadowBlur float32, shadowColor ui.Color) {
	renderNavSolidPillColor(ctx, key, rect, radius, host.AnimateColor(colorKey, bg, motionGlassMorph),
		pose, moveAnim, poseAnim, animateLayout, shadow, shadowBlur, shadowColor)
}

func renderNavSolidPillColor(ctx *ui.Context, key any, rect ui.Rect, radius float32, bg ui.Color, pose ui.Pose, moveAnim, poseAnim ui.Animation, animateLayout bool, shadow bool, shadowBlur float32, shadowColor ui.Color) {
	pill := rectBox(ctx.Key(key), rect, radius, 0).
		TransformOrigin(0.5, 0.5).AnimatePose(pose, poseAnim).
		Background(bg)
	if animateLayout {
		pill.Transition(ui.ElementTransition{Position: true, Size: true, Animation: &moveAnim})
	}
	if shadow && shadowColor.A > 0 {
		pill.Shadow(0, 1, shadowBlur, 0, shadowColor)
	}
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
	if isStaticSnapshot(ctx) || spec.Alpha <= 0.005 {
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

func compactNavLensExpand(mainExt, crossExt, elastic, stretch float32) (dx, dy, padCross float32) {
	e := max(0, elastic)
	mainEx := clamp32(mainExt*0.14, 4.5, 9.5)*e + mainExt*stretch*0.30
	crossEx := max(0, clamp32(crossExt*0.18, 3.2, 5.5)*e-crossExt*stretch*0.05)
	return mainEx, crossEx, crossEx
}

func navGlassBlend(active, scrubbing bool, cover, lensAlpha float32) float32 {
	switch {
	case scrubbing:
		return clamp32(cover*max(lensAlpha, 0.85), 0, 1)
	case active:
		return clamp32(cover, 0, 1)
	default:
		return clamp32(cover*lensAlpha, 0, 1)
	}
}

func renderNavGlassRestingPill(ctx *ui.Context, host ui.Element, key any, lens navGlassLens, radius float32, colorKey string, pillBg ui.Color, reduceTransparency, showShadow bool, shadowColor ui.Color) {
	renderNavGlassRestingPillColor(ctx, key, lens, radius, host.AnimateColor(colorKey, pillBg, motionGlassMorph), reduceTransparency, showShadow, shadowColor)
}

func renderNavGlassRestingPillColor(ctx *ui.Context, key any, lens navGlassLens, radius float32, bg ui.Color, reduceTransparency, showShadow bool, shadowColor ui.Color) {
	if !reduceTransparency {
		u := clamp32(1-1.25*lens.alpha, 0, 1)
		bg = scaleAlpha(bg, u*u*(3-2*u))
	}
	pill := rectBox(ctx.Key(key), lens.base, radius, 0).Background(bg)
	if showShadow && shadowColor.A > 0 {
		if fade := clamp32(1-2*lens.alpha, 0, 1); fade > 0 {
			pill.Shadow(0, 1, 2, 0, scaleAlpha(shadowColor, fade))
		}
	}
}

func renderNavGlassDroplet(ctx *ui.Context, key any, axis navScrubAxis, lens navGlassLens, baseRadius float32, dark bool) {
	lensRadius := max(1, baseRadius+max(lens.padCross, 0))
	lensEl := lens.box(ctx.Key(key), lensRadius, 3)
	navLensShadow(lensEl, dark, lens.alpha, false)
	zoomX, zoomY := 1+0.11*lens.alpha, 1+0.05*lens.alpha
	if axis == navAxisVertical {
		zoomX, zoomY = zoomY, zoomX
	}
	drawNavGlassLens(ctx, lensEl, lensRadius, liquidLensSpec{
		Alpha: lens.alpha, Glow: lens.glow, Touch: lens.touch, Zoom: zoomX, ZoomY: zoomY, ClearLens: true,
	})
}

type navTabVisualOpts struct {
	active, scrubbing, pressed, hovered, focused bool
	liquidGlass, glassOptics, lensMoving         bool
	solidPressDim                                bool
	cover, lensAlpha                             float32
	idleFg, activeFg, textFg, hoverBg, pressBg   ui.Color
	idleWeight, activeWeight                     int
}

type navTabVisual struct {
	fg, stateBg ui.Color
	weight      int
	colorAnim   ui.Animation
	iconPop     float32
}

func resolveNavTabVisual(o navTabVisualOpts) navTabVisual {
	v := navTabVisual{fg: o.idleFg, weight: o.idleWeight, colorAnim: motionNav}
	if o.active || (o.glassOptics && o.lensMoving && o.cover > 0.5) {
		v.weight = o.activeWeight
	}
	handled := false
	switch {
	case o.glassOptics && o.lensMoving:
		handled, v.colorAnim = true, MotionFast
		v.fg = o.idleFg.Mix(o.activeFg, navGlassBlend(o.active, o.scrubbing, o.cover, o.lensAlpha))
		v.iconPop = o.cover * o.lensAlpha
	case o.glassOptics && o.active:
		handled, v.fg = true, o.activeFg
		if o.pressed {
			v.iconPop = max(o.lensAlpha, 1)
		} else if o.lensAlpha > 0.01 {
			v.iconPop = o.cover * o.lensAlpha
		}
	case !o.glassOptics && o.active && !o.scrubbing && o.pressed:
		handled, v.fg = true, o.activeFg
		if o.solidPressDim && !o.liquidGlass {
			v.fg = o.activeFg.Alpha(0.90)
		}
	case !o.glassOptics && (o.active || (o.liquidGlass && o.lensMoving && o.cover > 0.5)):
		handled, v.fg = true, o.activeFg
		if o.lensMoving {
			v.colorAnim = MotionFast
		}
	}
	if !handled {
		allowHover := o.glassOptics || !o.scrubbing
		switch {
		case allowHover && o.pressed:
			v.stateBg, v.fg = o.pressBg, o.textFg
		case allowHover && (o.hovered || o.focused):
			v.stateBg, v.fg = o.hoverBg, o.textFg
		case o.scrubbing:
			v.colorAnim = MotionFast
		}
		if o.glassOptics && o.pressed {
			v.iconPop = 1
		}
	}
	return v
}

func resolveCompactSlots(ctx *ui.Context, slots []navSlot, itemEls []ui.Element, cfg compactGlassTrackConfig, hostBounds ui.Rect, modeChanged bool) (stripMain, stripCross, crossOff float32, measured bool) {
	if !modeChanged && hostBounds.W > 0 && hostBounds.H > 0 && len(itemEls) > 0 {
		allValid := true
		for i, el := range itemEls {
			b := el.LayoutBounds()
			if b.W <= 0 || b.H <= 0 {
				allValid = false
				break
			}
			slots[i] = navSlot{start: b.X - hostBounds.X, extent: b.W}
		}
		if allValid {
			b0 := itemEls[0].LayoutBounds()
			return hostBounds.W, b0.H, b0.Y - hostBounds.Y, true
		}
	}
	x := cfg.pad
	sel := clamp32Index(*cfg.selected, cfg.count)
	for i := 0; i < cfg.count; i++ {
		w := cfg.idleWeight
		if i == sel {
			w = cfg.activeWeight
		}
		tw, _ := ctx.MeasureText(1<<20, ui.Span{Text: cfg.labelAt(i), Size: cfg.fontSize, Weight: w})
		ext := tw + 2*cfg.padX
		slots[i] = navSlot{start: x, extent: ext}
		x += ext
		if i+1 < cfg.count {
			x += cfg.gap
		}
	}
	return x + cfg.pad, cfg.height, cfg.pad, false
}

type compactTrackScrubState struct {
	navScrubState
	navSlotCache
}

type compactGlassTrackConfig struct {
	keyPrefix                string
	mode                     uint8
	count                    int
	pad, gap, height, padX   float32
	radius, fontSize         float32
	idleWeight, activeWeight int
	restBg, solidBg          ui.Color
	activeFg                 ui.Color
	shadow, pressDim         bool
	checkSlot, requireArm    bool
	selected                 *int
	labelAt                  func(int) string
	buildItem                func(index int, st *navScrubState) ui.Element
	onCommit                 func(prev, target int)
}

func renderCompactGlassTrack(ctx *ui.Context, host ui.Element, p Palette, cfg compactGlassTrackConfig) {
	if cfg.count <= 0 {
		return
	}
	host.Draggable(nil, ui.DraggableOptions{Axis: ui.Horizontal, Bounds: &ui.Rect{}, Rigid: true})
	st := ui.Local(host, cfg.keyPrefix+"-scrub", func() compactTrackScrubState { return compactTrackScrubState{} })
	slots := st.slotsFor(cfg.count)
	host.ObserveInput(st.observePointer(navAxisHorizontal, cfg.gap, slots, *cfg.selected, cfg.count, cfg.checkSlot))
	hostBounds := host.LayoutBounds()

	host.Children(func() {
		var inlineEls [4]ui.Element
		els := inlineEls[:0]
		if cfg.count > len(inlineEls) {
			els = make([]ui.Element, 0, cfg.count)
		}
		for i := 0; i < cfg.count; i++ {
			els = append(els, cfg.buildItem(i, &st.navScrubState))
		}

		modeChanged := !st.hasPrev || st.lastMode != cfg.mode || st.lastCount != cfg.count
		if modeChanged {
			st.hasMeasured = false
		}
		stripMain, stripCross, crossOff, measured := resolveCompactSlots(ctx, slots, els, cfg, hostBounds, modeChanged)
		firstMeasure := measured && !st.hasMeasured
		jumpPill := modeChanged || firstMeasure

		prevSel := *cfg.selected
		displayIndex, _, committed, commitIdx := st.applyNavScrub(ctx, host, els, navAxisHorizontal, cfg.gap, slots, p.LiquidGlass, cfg.requireArm, cfg.selected)
		if committed && cfg.onCommit != nil {
			cfg.onCommit(prevSel, commitIdx)
		}

		pillStart, pillExtent := slots[displayIndex].start, slots[displayIndex].extent
		if st.active {
			pillStart = clamp32(st.lastX-pillExtent*0.5, cfg.pad, max(cfg.pad, stripMain-cfg.pad-pillExtent))
		}
		target := ui.Rect{X: pillStart, Y: crossOff, W: pillExtent, H: stripCross}

		reduceTransparency := ctx.Preferences().ReduceTransparency
		var lens navGlassLens
		if p.LiquidGlass {
			lens = st.animateGlassLens(ctx, host, navLensConfig{
				axis: navAxisHorizontal, target: target, mode: cfg.mode, count: cfg.count, keyPrefix: cfg.keyPrefix,
				style: navLensCompactStyle, tabPressed: els[displayIndex].Pressed(),
				forceJump: jumpPill, expand: compactNavLensExpand,
			})
			renderNavGlassRestingPill(ctx, host, cfg.keyPrefix+"-pill", lens, cfg.radius, cfg.keyPrefix+"-pill-bg", cfg.restBg, reduceTransparency, cfg.shadow, p.CardShadow)
		} else {
			st.hasPrev, st.lastMode, st.lastCount = true, cfg.mode, cfg.count
			pillBg, pillPose, pillAnim, pillPoseAnim := navSolidPillFeedback(
				st.active, els[displayIndex].Pressed(), st.lastX, stripMain, cfg.solidBg, p.Text, 0.08, 0.10,
			)
			renderNavSolidPill(ctx, host, cfg.keyPrefix+"-pill", target, cfg.radius, cfg.keyPrefix+"-pill-bg",
				pillBg, pillPose, pillAnim, pillPoseAnim,
				!jumpPill && !isStaticSnapshot(ctx) && !isScaleReflow(ctx),
				cfg.shadow && !els[displayIndex].Pressed() && !st.active, 2, p.CardShadow,
			)
		}
		if measured {
			st.hasMeasured = true
		}

		lensMoving := st.active || lens.inFlight
		glassOptics := p.LiquidGlass && !reduceTransparency
		hoverBg, pressBg := navInactiveOverlayColors(p, p.LiquidGlass)
		z := 1
		if glassOptics {
			z = 2
		}

		for i := 0; i < cfg.count; i++ {
			active := i == displayIndex
			el := els[i].ZIndex(z)
			cover, _ := lens.coverAt(navAxisHorizontal, slots[i].start+slots[i].extent*0.5, 0.85)
			vis := resolveNavTabVisual(navTabVisualOpts{
				active: active, scrubbing: st.active, pressed: el.Pressed(), hovered: el.Hovered(), focused: el.FocusVisible(),
				liquidGlass: p.LiquidGlass, glassOptics: glassOptics, lensMoving: lensMoving, solidPressDim: cfg.pressDim,
				cover: cover, lensAlpha: lens.alpha,
				idleFg: p.Muted, activeFg: cfg.activeFg, textFg: p.Text, hoverBg: hoverBg, pressBg: pressBg,
				idleWeight: cfg.idleWeight, activeWeight: cfg.activeWeight,
			})
			label := cfg.labelAt(i)
			el.Children(func() {
				if !active && !st.active {
					pillSurface(ctx.Key(cfg.keyPrefix+"-state"), el, cfg.radius, vis.stateBg, 0, ui.Transparent, 0.97)
				}
				AnimatedLabel(ctx, label).FontSize(cfg.fontSize).FontWeight(vis.weight).
					TextColor(el.AnimateColor(cfg.keyPrefix+"-fg", vis.fg, vis.colorAnim)).SingleLine().Shrink(0)
			})
		}

		if lens.optics {
			renderNavGlassDroplet(ctx, cfg.keyPrefix+"-pill-lens", navAxisHorizontal, lens, cfg.radius, p.Dark)
		}
	})
}
