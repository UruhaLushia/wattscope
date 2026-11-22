package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func inputDown(x, y float32) ui.InputEvent {
	return ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: x, Y: y}
}

func inputDownForAxis(axis navScrubAxis, main, cross float32) ui.InputEvent {
	if axis == navAxisVertical {
		return inputDown(cross, main)
	}
	return inputDown(main, cross)
}

func TestNavSlotIndexAtAndContainsVariableSizes(t *testing.T) {
	// Variable-width TopBar tabs with 2dp gap.
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
		{-25, 0},
		{0, 0},
		{88.5, 0},
		{89.1, 1},
		{154.5, 1},
		{155.1, 2},
		{258.5, 2},
		{259.1, 3},
		{500, 3},
	} {
		if got := navSlotIndexAt(tc.pos, 2, topSlots); got != tc.want {
			t.Fatalf("navSlotIndexAt(%.1f) = %d, want %d", tc.pos, got, tc.want)
		}
	}

	// Sidebar slots with expanded sub-items between slot 1 and slot 2.
	sideSlots := []navSlot{
		{start: 0, extent: 36},
		{start: 39, extent: 36},
		{start: 148, extent: 36},
	}
	if idx, ok := navSlotContains(15, 3, sideSlots); !ok || idx != 0 {
		t.Fatalf("slot 0 hit = (%d, %v), want (0, true)", idx, ok)
	}
	if idx, ok := navSlotContains(37.5, 3, sideSlots); !ok || idx != 0 {
		t.Fatalf("gap between 0 and 1 hit = (%d, %v), want (0, true)", idx, ok)
	}
	if idx, ok := navSlotContains(-10, 3, sideSlots); ok {
		t.Fatalf("outside toggle area should not hit, got (%d, true)", idx)
	}
	if idx, ok := navSlotContains(110, 3, sideSlots); ok {
		t.Fatalf("expanded sub-item area should not hit top-level slots, got (%d, true)", idx)
	}
	if idx, ok := navSlotContains(160, 3, sideSlots); !ok || idx != 2 {
		t.Fatalf("slot 2 hit = (%d, %v), want (2, true)", idx, ok)
	}
	if got := navSlotIndexAt(160, 3, sideSlots); got != 2 {
		t.Fatalf("navSlotIndexAt(160) = %d, want 2", got)
	}
}

func TestStepNavScrubShortTapDoesNotScrub(t *testing.T) {
	slots := []navSlot{{start: 0, extent: 70}, {start: 72, extent: 70}, {start: 144, extent: 70}}
	t0 := time.Unix(100, 0)
	var st navScrubState

	// Pointer down on tab 1 while tab 0 is selected.
	st.observePointer(navAxisHorizontal, 2, slots, 0, len(slots), true)(inputDown(80, 12))
	out := stepNavScrub(&st, navScrubInput{
		now: t0, x: 80, y: 12, mainPos: 80,
		anyPressed: true, dragThresh: navScrubDragThresh, holdDelay: navScrubHoldDelay,
		selected: 0, slots: slots, gap: 2, allowArm: true,
	})
	if st.active || out.commit {
		t.Fatalf("short press frame 0 unexpectedly activated: st=%+v out=%+v", st, out)
	}
	if out.waitForHold != navScrubHoldDelay {
		t.Fatalf("waitForHold = %v, want %v", out.waitForHold, navScrubHoldDelay)
	}

	// Release after 50ms with small jitter < dragThresh: normal OnClick should handle it.
	out = stepNavScrub(&st, navScrubInput{
		now: t0.Add(50 * time.Millisecond), x: 84, y: 14, mainPos: 84,
		released: true, dragThresh: navScrubDragThresh, holdDelay: navScrubHoldDelay,
		selected: 0, slots: slots, gap: 2,
	})
	if out.commit || st.suppressClick || st.pressing || st.active {
		t.Fatalf("short tap release should not scrub-commit or suppressClick: st=%+v out=%+v", st, out)
	}
}

func TestStepNavScrubHorizontalAndVerticalDragCommit(t *testing.T) {
	slots := []navSlot{{start: 0, extent: 50}, {start: 52, extent: 70}, {start: 124, extent: 60}}
	t0 := time.Unix(200, 0)

	for _, axis := range []navScrubAxis{navAxisHorizontal, navAxisVertical} {
		var st navScrubState
		st.observePointer(axis, 2, slots, 0, len(slots), true)(inputDownForAxis(axis, 20, 8))
		stepNavScrub(&st, navScrubInput{
			now: t0, x: 20, y: 8, mainPos: 20,
			anyPressed: true, dragThresh: navScrubDragThresh, holdDelay: navScrubHoldDelay,
			selected: 0, slots: slots, gap: 2, allowArm: true,
		})

		// Drag past threshold + large cross-axis drift should stay active and clamp to last slot.
		x, y := float32(240), float32(180)
		mainPos := navMainCoord(axis, x, y)
		if axis == navAxisVertical {
			x, y = 180, 240
			mainPos = navMainCoord(axis, x, y)
		}
		out := stepNavScrub(&st, navScrubInput{
			now: t0.Add(24 * time.Millisecond), x: x, y: y, mainPos: mainPos,
			anyPressed: true, dragThresh: navScrubDragThresh, holdDelay: navScrubHoldDelay,
			selected: 0, slots: slots, gap: 2, allowArm: true,
		})
		if !st.active || !out.activeChanged || st.dragIndex != 2 {
			t.Fatalf("axis %v: expected active drag at index 2, got st=%+v out=%+v", axis, st, out)
		}

		// Release commits and sets suppressClick.
		out = stepNavScrub(&st, navScrubInput{
			now: t0.Add(40 * time.Millisecond), x: x, y: y, mainPos: mainPos,
			released: true, dragThresh: navScrubDragThresh, holdDelay: navScrubHoldDelay,
			selected: 0, slots: slots, gap: 2,
		})
		if !out.commit || out.commitIndex != 2 || !st.suppressClick {
			t.Fatalf("axis %v: expected commit index 2 and suppressClick, got st=%+v out=%+v", axis, st, out)
		}

		// Next idle frame clears suppressClick.
		stepNavScrub(&st, navScrubInput{
			now: t0.Add(56 * time.Millisecond), dragThresh: navScrubDragThresh, holdDelay: navScrubHoldDelay,
			selected: 2, slots: slots, gap: 2,
		})
		if st.suppressClick {
			t.Fatalf("axis %v: expected suppressClick to clear on idle frame", axis)
		}
	}
}

func TestStepNavScrubStationaryHold180msActivatesAndFollows(t *testing.T) {
	slots := []navSlot{{start: 0, extent: 60}, {start: 62, extent: 60}, {start: 124, extent: 60}}
	t0 := time.Unix(300, 0)
	var st navScrubState

	// Press stationary on tab 2 (while tab 0 is selected).
	st.observePointer(navAxisHorizontal, 2, slots, 0, len(slots), true)(inputDown(150, 14))
	out := stepNavScrub(&st, navScrubInput{
		now: t0, x: 150, y: 14, mainPos: 150,
		anyPressed: true, dragThresh: navScrubDragThresh, holdDelay: navScrubHoldDelay,
		selected: 0, slots: slots, gap: 2, allowArm: true,
	})
	if st.active || out.waitForHold != navScrubHoldDelay {
		t.Fatalf("expected waitForHold=%v, got st=%+v out=%+v", navScrubHoldDelay, st, out)
	}

	// Halfway (100ms): still waiting.
	out = stepNavScrub(&st, navScrubInput{
		now: t0.Add(100 * time.Millisecond), x: 150, y: 14, mainPos: 150,
		anyPressed: true, dragThresh: navScrubDragThresh, holdDelay: navScrubHoldDelay,
		selected: 0, slots: slots, gap: 2, allowArm: true,
	})
	if st.active || out.waitForHold != 80*time.Millisecond {
		t.Fatalf("at 100ms expected waitForHold=80ms, got st=%+v out=%+v", st, out)
	}

	// At 180ms without movement: activates scrub immediately onto held tab 2!
	out = stepNavScrub(&st, navScrubInput{
		now: t0.Add(navScrubHoldDelay), x: 150, y: 14, mainPos: 150,
		anyPressed: true, dragThresh: navScrubDragThresh, holdDelay: navScrubHoldDelay,
		selected: 0, slots: slots, gap: 2, allowArm: true,
	})
	if !st.active || !out.activeChanged || st.dragIndex != 2 {
		t.Fatalf("at 180ms expected active dragIndex=2, got st=%+v out=%+v", st, out)
	}

	// Subsequent drag onto tab 1 updates dragIndex.
	stepNavScrub(&st, navScrubInput{
		now: t0.Add(220 * time.Millisecond), x: 90, y: 14, mainPos: 90,
		anyPressed: true, dragThresh: navScrubDragThresh, holdDelay: navScrubHoldDelay,
		selected: 0, slots: slots, gap: 2, allowArm: true,
	})
	if st.dragIndex != 1 {
		t.Fatalf("expected dragIndex=1 after sliding, got %d", st.dragIndex)
	}
}

func TestStepNavScrubSystemCancelAndNonSlotGuard(t *testing.T) {
	slots := []navSlot{{start: 0, extent: 36}, {start: 39, extent: 36}, {start: 140, extent: 36}}
	t0 := time.Unix(400, 0)
	var st navScrubState

	// Pressing in expanded sub-item region (y=90) or outside slots (y=-15) does not arm.
	st.observePointer(navAxisVertical, 3, slots, 0, len(slots), true)(inputDown(18, 90))
	if st.pressing {
		t.Fatal("pointer down outside top-level slots should not arm pressing")
	}
	stepNavScrub(&st, navScrubInput{
		now: t0, x: 18, y: 90, mainPos: 90,
		anyPressed: true, dragThresh: navScrubDragThresh, holdDelay: navScrubHoldDelay,
		selected: 0, slots: slots, gap: 3, allowArm: false,
	})
	if st.pressing || st.active {
		t.Fatalf("allowArm=false must not arm scrub: %+v", st)
	}

	// Valid press -> activate -> system cancel reverts without commit.
	st.observePointer(navAxisVertical, 3, slots, 0, len(slots), true)(inputDown(18, 12))
	stepNavScrub(&st, navScrubInput{
		now: t0, x: 18, y: 12, mainPos: 12,
		anyPressed: true, dragThresh: navScrubDragThresh, holdDelay: navScrubHoldDelay,
		selected: 0, slots: slots, gap: 3, allowArm: true,
	})
	stepNavScrub(&st, navScrubInput{
		now: t0.Add(20 * time.Millisecond), x: 18, y: 155, mainPos: 155,
		anyPressed: true, dragThresh: navScrubDragThresh, holdDelay: navScrubHoldDelay,
		selected: 0, slots: slots, gap: 3, allowArm: true,
	})
	if !st.active || st.dragIndex != 2 {
		t.Fatalf("expected active scrub before cancel, got %+v", st)
	}
	out := stepNavScrub(&st, navScrubInput{
		now: t0.Add(30 * time.Millisecond), x: 18, y: 155, mainPos: 155,
		anyCanceled: true, released: true, dragThresh: navScrubDragThresh, holdDelay: navScrubHoldDelay,
		selected: 0, slots: slots, gap: 3,
	})
	if out.commit || st.pressing || st.active || st.suppressClick {
		t.Fatalf("canceled scrub must not commit: st=%+v out=%+v", st, out)
	}
}
