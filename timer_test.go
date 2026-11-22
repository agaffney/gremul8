package gremul8

import (
	"bytes"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tests for the delay and sound timers and for the machine's ticking. The FX07,
// FX15 and FX18 opcodes themselves are tested in opcode_test.go.

// timers returns the timer values, taking the machine's lock so that it is safe
// to call while an automatic ticker is running.
func timers(m *Machine) (delay, sound uint8) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.delayTimer, m.soundTimer
}

func setTimers(m *Machine, delay, sound uint8) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delayTimer, m.soundTimer = delay, sound
}

// newMachineWithOpts is newMachineWithROM, with options for the machine.
func newMachineWithOpts(t *testing.T, rom []byte, opts ...Option) *Machine {
	t.Helper()
	m := NewMachine(opts...)
	t.Cleanup(m.Close)
	if err := m.Load(bytes.NewReader(rom)); err != nil {
		t.Fatalf("loading program: %s", err)
	}
	return m
}

func wantTimers(t *testing.T, m *Machine, wantDelay, wantSound uint8) {
	t.Helper()
	if d, s := timers(m); d != wantDelay || s != wantSound {
		t.Errorf("timers (delay, sound) = (%d, %d), want (%d, %d)", d, s, wantDelay, wantSound)
	}
}

// eventually polls cond until it returns true, failing the test if it still has
// not after timeout. Tests of the automatic ticker use it instead of one fixed
// sleep followed by an exact check, so that a slow or stalled machine only makes
// them slower, not flaky. (Checks that something does NOT happen can't work this
// way and still use a fixed sleep.)
func eventually(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", timeout, what)
		}
		time.Sleep(TickDuration / 4)
	}
}

// How long the automatic-ticker tests will wait for something that should take a
// few tick periods. It is far longer than needed on a healthy machine.
const tickWait = 3 * time.Second

// ---------------------------------------------------------------------------
// Tick
// ---------------------------------------------------------------------------

func TestTickDecrementsBothTimers(t *testing.T) {
	m := NewMachine()
	setTimers(m, 5, 3)
	m.Tick()
	wantTimers(t, m, 4, 2)
	m.Tick()
	wantTimers(t, m, 3, 1)
}

// A timer that is already zero stays at zero. It must not wrap round to 255 and
// start counting down again, which would make a ROM that waits on the timer hang
// and make the sound play forever.
func TestTickStopsAtZero(t *testing.T) {
	m := NewMachine()
	setTimers(m, 0, 0)
	for range 3 {
		m.Tick()
		wantTimers(t, m, 0, 0)
	}
}

func TestTickCountsDownToZeroAndStops(t *testing.T) {
	m := NewMachine()
	setTimers(m, 3, 3)
	for range 3 {
		m.Tick()
	}
	wantTimers(t, m, 0, 0)
	m.Tick()
	wantTimers(t, m, 0, 0)
}

// Each timer is independent: one running out must not affect the other.
func TestTickTimersAreIndependent(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		delay, sound         uint8
		wantDelay, wantSound uint8
	}{
		{"only delay running", 4, 0, 3, 0},
		{"only sound running", 0, 4, 0, 3},
		{"delay about to expire", 1, 5, 0, 4},
		{"sound about to expire", 5, 1, 4, 0},
		{"full values", 255, 255, 254, 254},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewMachine()
			setTimers(m, tc.delay, tc.sound)
			m.Tick()
			wantTimers(t, m, tc.wantDelay, tc.wantSound)
		})
	}
}

func TestTickDoesNotTouchOtherState(t *testing.T) {
	m := newMachineWithROM(t, []byte{0xa3, 0x45})
	for i := range m.regV {
		m.regV[i] = uint8(i + 1)
	}
	step(t, m) // LD I, 0x345
	setTimers(m, 9, 9)
	regs, i, pc, sp, mem := m.regV, m.regI, m.pc, m.sp, m.mem
	m.Tick()
	if m.regV != regs || m.regI != i || m.pc != pc || m.sp != sp || m.mem != mem {
		t.Errorf("Tick changed registers, I, pc, sp or memory")
	}
}

// A program that sets the delay timer and reads it back after some ticks sees the
// count go down.
func TestTimerCountdownSeenByProgram(t *testing.T) {
	m := newMachineWithROM(t, []byte{
		0x60, 0x05, // LD V0, 5
		0xf0, 0x15, // LD DT, V0
		0xf1, 0x07, // LD V1, DT
	})
	step(t, m)
	step(t, m)
	m.Tick()
	m.Tick()
	step(t, m)
	if m.regV[1] != 3 {
		t.Errorf("V1 = %d, want 3 (5 minus two ticks)", m.regV[1])
	}
}

// ---------------------------------------------------------------------------
// Reset
// ---------------------------------------------------------------------------

func TestTimersStartAtZero(t *testing.T) {
	wantTimers(t, NewMachine(), 0, 0)
}

func TestLoadClearsTimers(t *testing.T) {
	m := NewMachine()
	setTimers(m, 200, 100)
	if err := m.Load(bytes.NewReader([]byte{0x12, 0x00})); err != nil {
		t.Fatal(err)
	}
	wantTimers(t, m, 0, 0)
}

// ---------------------------------------------------------------------------
// Automatic ticking
// ---------------------------------------------------------------------------

// CHIP-8's timers count down at 60Hz.
func TestTickDurationIsSixtyHertz(t *testing.T) {
	if TickDuration < 16*time.Millisecond || TickDuration > 17*time.Millisecond {
		t.Errorf("TickDuration = %s, want about 16.7ms (1/60 s)", TickDuration)
	}
}

// Setting both timers to their maximum from a program, so the tests below are
// going through the same path a ROM would.
var setBothTimersMax = []byte{
	0x60, 0xff, // LD V0, 0xff
	0xf0, 0x15, // LD DT, V0
	0xf0, 0x18, // LD ST, V0
}

func TestNoAutoTickByDefault(t *testing.T) {
	m := newMachineWithOpts(t, setBothTimersMax)
	for range 3 {
		step(t, m)
	}
	time.Sleep(3 * TickDuration)
	wantTimers(t, m, 0xff, 0xff)
}

func TestAutoTickFalseDoesNotTick(t *testing.T) {
	m := newMachineWithOpts(t, setBothTimersMax, WithAutoTick(false))
	for range 3 {
		step(t, m)
	}
	time.Sleep(3 * TickDuration)
	wantTimers(t, m, 0xff, 0xff)
}

// With WithAutoTick, the timers count down on their own, together, and keep
// doing so.
func TestAutoTickCountsDownRepeatedly(t *testing.T) {
	m := newMachineWithOpts(t, setBothTimersMax, WithAutoTick(true))
	for range 3 {
		step(t, m)
	}
	var d1, s1 uint8
	eventually(t, tickWait, "both timers to tick down at least once", func() bool {
		d1, s1 = timers(m)
		return d1 < 0xff && s1 < 0xff
	})
	// It must go on ticking, not just fire once
	eventually(t, tickWait, "the timers to keep counting down", func() bool {
		d2, s2 := timers(m)
		return d2 < d1 && s2 < s1
	})
}

// An automatic ticker must also stop at zero, and stay there.
func TestAutoTickStopsAtZero(t *testing.T) {
	m := newMachineWithOpts(t, []byte{
		0x60, 0x02, // LD V0, 2
		0xf0, 0x15, // LD DT, V0
		0xf0, 0x18, // LD ST, V0
	}, WithAutoTick(true))
	for range 3 {
		step(t, m)
	}
	eventually(t, tickWait, "both timers to reach zero", func() bool {
		d, s := timers(m)
		return d == 0 && s == 0
	})
	// Give it a few more periods in which to wrongly wrap round
	time.Sleep(4 * TickDuration)
	wantTimers(t, m, 0, 0)
}

// Loading a program resets the machine, and the timers with it; the old ticker
// must not carry on counting into the new run, and the machine must not collect a
// goroutine for every Load.
func TestAutoTickLoadClearsTimersAndDoesNotLeak(t *testing.T) {
	m := newMachineWithOpts(t, setBothTimersMax, WithAutoTick(true))
	runtime.GC()
	before := runtime.NumGoroutine()
	for range 50 {
		if err := m.Load(bytes.NewReader(setBothTimersMax)); err != nil {
			t.Fatal(err)
		}
		wantTimers(t, m, 0, 0)
	}
	time.Sleep(TickDuration)
	if after := runtime.NumGoroutine(); after > before+5 {
		t.Errorf("goroutines went from %d to %d over 50 Loads: tickers are leaking", before, after)
	}
}

// A machine that has just been created is already reset: font in memory, pc at
// the start of the program area, display clear.
func TestNewMachineIsReset(t *testing.T) {
	m := NewMachine()
	if m.pc != RomAddress {
		t.Errorf("pc = %03x, want %03x", m.pc, RomAddress)
	}
	if !fontIntact(m) {
		t.Errorf("built-in font is not in memory")
	}
	wantTimers(t, m, 0, 0)
}

// WithAutoTick takes effect as soon as the machine exists, without a Load.
func TestAutoTickRunsWithoutLoad(t *testing.T) {
	m := NewMachine(WithAutoTick(true))
	t.Cleanup(m.Close)
	setTimers(m, 200, 200)
	eventually(t, tickWait, "the timers to tick down", func() bool {
		d, s := timers(m)
		return d < 200 && s < 200
	})
}

// Load must be atomic with respect to Step: a Step on another goroutine may see
// the old program or the new one, but never the zeroed memory in between. The
// program here is a jump to itself, so a Step that sees zeroed memory fails with
// "unknown instruction: 0000". Run this under -race as well.
func TestLoadIsAtomicWithStep(t *testing.T) {
	for _, auto := range []bool{false, true} {
		name := "manual ticks"
		if auto {
			name = "auto ticks"
		}
		t.Run(name, func(t *testing.T) {
			rom := []byte{0x12, 0x00} // 0x200: JP 0x200
			m := newMachineWithOpts(t, rom, WithAutoTick(auto))

			var done atomic.Bool
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer done.Store(true)
				for range 3000 {
					if err := m.Load(bytes.NewReader(rom)); err != nil {
						t.Errorf("Load: %s", err)
						return
					}
				}
			}()

			var stepErr error
			steps := 0
			for !done.Load() {
				if err := m.Step(); err != nil {
					stepErr = err
					break
				}
				steps++
			}
			done.Store(true)
			wg.Wait()
			if stepErr != nil {
				t.Errorf("Step failed during a concurrent Load (after %d steps): %s", steps, stepErr)
			}
		})
	}
}

// Steps, Loads and automatic ticks all run on different goroutines here. Under
// -race this catches unsynchronised access to the timers and the ticker; the
// assertions check that a Load always leaves the timers cleared.
func TestAutoTickConcurrentWithStepAndLoad(t *testing.T) {
	rom := []byte{
		0x60, 0xff, // LD V0, 0xff
		0xf0, 0x15, // LD DT, V0
		0xf1, 0x07, // LD V1, DT
		0x12, 0x04, // JP 0x204
	}
	m := newMachineWithOpts(t, rom, WithAutoTick(true))
	deadline := time.Now().Add(3 * TickDuration)
	for time.Now().Before(deadline) {
		step(t, m)
	}
	if d, _ := timers(m); d == 0 {
		t.Errorf("delay timer is 0 after running a program that set it to 255")
	}
	for range 3 {
		if err := m.Load(bytes.NewReader(rom)); err != nil {
			t.Fatal(err)
		}
		wantTimers(t, m, 0, 0)
		time.Sleep(TickDuration)
		for range 4 {
			step(t, m)
		}
	}
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestCloseIsIdempotent(t *testing.T) {
	for _, auto := range []bool{false, true} {
		m := NewMachine(WithAutoTick(auto))
		for range 3 {
			if !noPanic(t, "Close", m.Close) {
				return
			}
		}
	}
}

// Once closed, a machine refuses to load another program, whether or not it was
// auto-ticking, and the refused Load leaves it as it was.
func TestLoadAfterCloseFails(t *testing.T) {
	for _, auto := range []bool{false, true} {
		name := "no auto tick"
		if auto {
			name = "auto tick"
		}
		t.Run(name, func(t *testing.T) {
			m := newMachineWithOpts(t, []byte{0x12, 0x00}, WithAutoTick(auto))
			m.mu.Lock()
			m.regV[0] = 7
			m.mu.Unlock()
			m.Close()
			err := m.Load(bytes.NewReader([]byte{0x12, 0x00}))
			wantErrContaining(t, err, "Close()")
			m.mu.Lock()
			v0 := m.regV[0]
			m.mu.Unlock()
			if v0 != 7 {
				t.Errorf("a refused Load reset the machine: V0 = %d, want 7", v0)
			}
		})
	}
}

// Closing a machine must stop its ticker goroutine, so that creating and closing
// machines repeatedly doesn't pile up goroutines. Without Close, each auto-ticking
// machine's goroutine lives for the life of the process.
func TestCloseStopsTickerGoroutine(t *testing.T) {
	runtime.GC()
	before := runtime.NumGoroutine()
	for range 50 {
		NewMachine(WithAutoTick(true)).Close()
	}
	eventually(t, tickWait, "the ticker goroutines to exit", func() bool {
		return runtime.NumGoroutine() <= before+5
	})
}

// After Close the timers no longer count down by themselves. One tick that was
// already on its way when Close was called may still land, so allow for one.
// This checks the observable behaviour only: Stop() alone would silence the
// ticker, so goroutine exit is covered by TestCloseStopsTickerGoroutine.
func TestCloseStopsAutoTicking(t *testing.T) {
	m := NewMachine(WithAutoTick(true))
	m.Close()
	setTimers(m, 200, 200)
	time.Sleep(6 * TickDuration)
	if d, s := timers(m); d < 199 || s < 199 {
		t.Errorf("timers kept counting after Close: (%d, %d), want at least (199, 199)", d, s)
	}
}

// Close doesn't disturb the machine's state or stop it being stepped.
func TestCloseLeavesMachineUsable(t *testing.T) {
	m := newMachineWithOpts(t, []byte{0x60, 0x2a}, WithAutoTick(true)) // LD V0, 0x2a
	m.Close()
	step(t, m)
	m.mu.Lock()
	v0 := m.regV[0]
	m.mu.Unlock()
	if v0 != 0x2a {
		t.Errorf("V0 = %02x after Close and a Step, want 2a", v0)
	}
}
