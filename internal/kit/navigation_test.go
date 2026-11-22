package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func inputDownForAxis(axis navScrubAxis, main, cross float32) ui.InputEvent {
	if axis == navAxisVertical {
		return ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: cross, Y: main}
	}
	return ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: main, Y: cross}
}

func TestNavSlotIndexAtAndContains(t *testing.T) {
	topSlots := []navSlot{
		{start: 0, extent: 88},
		{start: 90, extent: 64},
		{start: 156, extent: 102},
		{start: 260, extent: 76},
	}
	for _, tc := range []struct {
		pos  float32
		want int
	}{
		{-25, 0}, {88.5, 0}, {89.1, 1}, {155.1, 2}, {259.1, 3}, {500, 3},
	} {
		if got := navSlotIndexAt(tc.pos, 2, topSlots); got != tc.want {
			t.Fatalf("navSlotIndexAt(%.1f) = %d, want %d", tc.pos, got, tc.want)
		}
	}

	sideSlots := []navSlot{{start: 0, extent: 36}, {start: 39, extent: 36}, {start: 148, extent: 36}}
	if idx, ok := navSlotContains(37.5, 3, sideSlots); !ok || idx != 0 {
		t.Fatalf("gap hit = (%d, %v), want (0, true)", idx, ok)
	}
	if _, ok := navSlotContains(-10, 3, sideSlots); ok {
		t.Fatal("outside toggle area should not hit")
	}
	if _, ok := navSlotContains(110, 3, sideSlots); ok {
		t.Fatal("expanded gap area should not hit")
	}
	if idx, ok := navSlotContains(160, 3, sideSlots); !ok || idx != 2 {
		t.Fatalf("slot 2 hit = (%d, %v), want (2, true)", idx, ok)
	}
}

func TestStepNavScrub(t *testing.T) {
	t0 := time.Unix(100, 0)
	base := func(now time.Time, main float32, slots []navSlot) navScrubInput {
		return navScrubInput{
			now: now, x: main, y: 12, mainPos: main,
			dragThresh: navScrubDragThresh, holdDelay: navScrubHoldDelay,
			slots: slots, gap: 2, allowArm: true,
		}
	}

	t.Run("short tap does not scrub", func(t *testing.T) {
		slots := []navSlot{{start: 0, extent: 70}, {start: 72, extent: 70}, {start: 144, extent: 70}}
		var st navScrubState
		st.observePointer(navAxisHorizontal, 2, slots, 0, len(slots), true)(inputDownForAxis(navAxisHorizontal, 80, 12))
		in := base(t0, 80, slots)
		in.anyPressed = true
		if out := stepNavScrub(&st, in); st.active || out.commit || out.waitForHold != navScrubHoldDelay {
			t.Fatalf("unexpected frame 0: st=%+v out=%+v", st, out)
		}
		rel := base(t0.Add(50*time.Millisecond), 84, slots)
		rel.released = true
		if out := stepNavScrub(&st, rel); out.commit || st.suppressClick || st.pressing || st.active {
			t.Fatalf("short tap release should not commit: st=%+v out=%+v", st, out)
		}
	})

	t.Run("horizontal and vertical drag commit", func(t *testing.T) {
		slots := []navSlot{{start: 0, extent: 50}, {start: 52, extent: 70}, {start: 124, extent: 60}}
		for _, axis := range []navScrubAxis{navAxisHorizontal, navAxisVertical} {
			var st navScrubState
			st.observePointer(axis, 2, slots, 0, len(slots), true)(inputDownForAxis(axis, 20, 8))
			in := base(t0, 20, slots)
			in.anyPressed = true
			stepNavScrub(&st, in)

			drag := base(t0.Add(24*time.Millisecond), 240, slots)
			drag.anyPressed = true
			if out := stepNavScrub(&st, drag); !st.active || !out.activeChanged || st.dragIndex != 2 {
				t.Fatalf("axis %v drag failed: st=%+v out=%+v", axis, st, out)
			}
			rel := base(t0.Add(40*time.Millisecond), 240, slots)
			rel.released = true
			if out := stepNavScrub(&st, rel); !out.commit || out.commitIndex != 2 || !st.suppressClick {
				t.Fatalf("axis %v commit failed: st=%+v out=%+v", axis, st, out)
			}
			stepNavScrub(&st, base(t0.Add(56*time.Millisecond), 0, slots))
			if st.suppressClick {
				t.Fatalf("axis %v suppressClick did not clear on idle", axis)
			}
		}
	})

	t.Run("stationary hold activates after 180ms and follows", func(t *testing.T) {
		slots := []navSlot{{start: 0, extent: 60}, {start: 62, extent: 60}, {start: 124, extent: 60}}
		var st navScrubState
		st.observePointer(navAxisHorizontal, 2, slots, 0, len(slots), true)(inputDownForAxis(navAxisHorizontal, 150, 14))
		in := base(t0, 150, slots)
		in.anyPressed = true
		stepNavScrub(&st, in)
		in.now = t0.Add(100 * time.Millisecond)
		if out := stepNavScrub(&st, in); st.active || out.waitForHold != 80*time.Millisecond {
			t.Fatalf("at 100ms got st=%+v out=%+v", st, out)
		}
		in.now = t0.Add(navScrubHoldDelay)
		if out := stepNavScrub(&st, in); !st.active || !out.activeChanged || st.dragIndex != 2 {
			t.Fatalf("at 180ms got st=%+v out=%+v", st, out)
		}
		in.now, in.mainPos = t0.Add(220*time.Millisecond), 30
		if out := stepNavScrub(&st, in); !st.active || out.activeChanged || st.dragIndex != 0 {
			t.Fatalf("post-hold drag got st=%+v out=%+v", st, out)
		}
		in.now, in.anyPressed, in.released = t0.Add(240*time.Millisecond), false, true
		if out := stepNavScrub(&st, in); !out.commit || out.commitIndex != 0 {
			t.Fatalf("post-hold release got st=%+v out=%+v", st, out)
		}
	})

	t.Run("non-slot, allowArm, and cancel guards", func(t *testing.T) {
		slots := []navSlot{{start: 0, extent: 36}, {start: 39, extent: 36}, {start: 140, extent: 36}}
		var st navScrubState
		st.observePointer(navAxisVertical, 3, slots, 0, len(slots), true)(inputDownForAxis(navAxisVertical, 90, 18))
		if st.pressing {
			t.Fatal("pointer outside slots should not arm")
		}
		noArm := base(t0, 90, slots)
		noArm.gap, noArm.anyPressed, noArm.allowArm = 3, true, false
		if out := stepNavScrub(&st, noArm); st.pressing || st.active || out.waitForHold != 0 {
			t.Fatalf("allowArm=false should not arm: st=%+v out=%+v", st, out)
		}
		st.observePointer(navAxisVertical, 3, slots, 0, len(slots), true)(inputDownForAxis(navAxisVertical, 12, 18))
		in := base(t0, 12, slots)
		in.gap, in.anyPressed = 3, true
		stepNavScrub(&st, in)
		in.now, in.mainPos = t0.Add(20*time.Millisecond), 155
		stepNavScrub(&st, in)
		in.now, in.anyPressed, in.anyCanceled, in.released = t0.Add(30*time.Millisecond), false, true, true
		if out := stepNavScrub(&st, in); out.commit || st.pressing || st.active || st.suppressClick {
			t.Fatalf("canceled scrub must not commit: st=%+v out=%+v", st, out)
		}
	})
}
