package backend

import (
	"math"
	"testing"
	"time"

	"github.com/UruhaLushia/wattscope/internal/device"
)

func TestCurrentDirectionTrackerLatchesAcrossLowCurrent(t *testing.T) {
	base := time.Unix(100, 0)
	var tracker currentDirectionTracker
	sample := func(voltage, current float64, direction device.CurrentDirection) *device.Sample {
		return &device.Sample{Voltage: voltage, Current: current, CurrentDirection: direction}
	}
	if tracker.update(sample(19.9, 0.008, device.CurrentPlugToReceptacle), base) {
		t.Fatal("direction stabilized on the first sample")
	}
	if !tracker.update(sample(19.9, 0.008, device.CurrentPlugToReceptacle), base.Add(directionConfirmDuration)) {
		t.Fatal("direction did not stabilize after confirmation")
	}
	if tracker.direction != device.CurrentPlugToReceptacle {
		t.Fatalf("locked direction = %q", tracker.direction)
	}
	for _, current := range []float64{0.002, 0, 0.003} {
		frame := sample(19.9, current, device.CurrentReceptacleToPlug)
		tracker.apply(frame, base.Add(400*time.Millisecond))
		if !frame.CurrentDirectionStable {
			t.Fatalf("low-current frame cleared the lock at %.3f A", current)
		}
		if frame.CurrentDirection != device.CurrentPlugToReceptacle {
			t.Fatalf("low-current frame flipped direction at %.3f A", current)
		}
	}
}

func TestCurrentDirectionTrackerIgnoresNearZeroNoiseBeforeLock(t *testing.T) {
	base := time.Unix(200, 0)
	var tracker currentDirectionTracker
	frames := []struct {
		current   float64
		direction device.CurrentDirection
	}{
		{0.002, device.CurrentPlugToReceptacle},
		{0.003, device.CurrentReceptacleToPlug},
		{0.002, device.CurrentPlugToReceptacle},
		{0.003, device.CurrentReceptacleToPlug},
	}
	for index, frame := range frames {
		if tracker.update(&device.Sample{
			Voltage:          19.9,
			Current:          frame.current,
			CurrentDirection: frame.direction,
		}, base.Add(time.Duration(index)*directionConfirmDuration)) {
			t.Fatal("near-zero noise established a direction")
		}
	}
}

func TestCurrentDirectionTrackerSwitchesOnConfirmedReverse(t *testing.T) {
	base := time.Unix(300, 0)
	var tracker currentDirectionTracker
	initial := device.Sample{Voltage: 19.9, Current: 0.01, CurrentDirection: device.CurrentPlugToReceptacle}
	tracker.update(&initial, base)
	if !tracker.update(&initial, base.Add(directionConfirmDuration)) {
		t.Fatal("initial direction did not stabilize")
	}
	reverse := device.Sample{Voltage: 19.9, Current: directionReverseCurrent, CurrentDirection: device.CurrentReceptacleToPlug}
	if !tracker.update(&reverse, base.Add(300*time.Millisecond)) {
		t.Fatal("reverse candidate cleared the existing lock")
	}
	if tracker.direction != device.CurrentPlugToReceptacle {
		t.Fatal("reverse direction switched before confirmation")
	}
	if !tracker.update(&reverse, base.Add(300*time.Millisecond+directionConfirmDuration)) {
		t.Fatal("confirmed reverse direction was not reported stable")
	}
	if tracker.direction != device.CurrentReceptacleToPlug {
		t.Fatalf("direction after reverse = %q", tracker.direction)
	}
}

func TestCurrentDirectionTrackerClearsWhenPowerDisappears(t *testing.T) {
	base := time.Unix(400, 0)
	var tracker currentDirectionTracker
	initial := device.Sample{Voltage: 19.9, Current: 0.01, CurrentDirection: device.CurrentPlugToReceptacle}
	tracker.update(&initial, base)
	if !tracker.update(&initial, base.Add(directionConfirmDuration)) {
		t.Fatal("initial direction did not stabilize")
	}
	if tracker.update(&device.Sample{Voltage: directionVoltageOff, Current: 0, CurrentDirection: device.CurrentPlugToReceptacle}, base.Add(500*time.Millisecond)) {
		t.Fatal("direction remained stable after power loss")
	}
	if tracker.direction != "" || tracker.powered {
		t.Fatalf("tracker after power loss = %+v", tracker)
	}
	if tracker.update(&device.Sample{Voltage: math.NaN(), Current: 0, CurrentDirection: device.CurrentPlugToReceptacle}, base.Add(time.Second)) {
		t.Fatal("non-finite voltage restored direction")
	}
}
