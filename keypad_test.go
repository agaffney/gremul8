package gremul8

import (
	"bytes"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tests for the keypad: KeyDown/KeyUp, the Ex9E and ExA1 skips, and Fx0A, which
// waits for a key. The test-suite ROMs that need a keypad are at the end.

// setKey presses or releases a key from a test, going through the public methods.
func setKey(m *Machine, k uint8, down bool) {
	if down {
		m.KeyDown(k)
	} else {
		m.KeyUp(k)
	}
}

// ---------------------------------------------------------------------------
// KeyDown / KeyUp
// ---------------------------------------------------------------------------

func TestKeyDownUpEveryKey(t *testing.T) {
	for k := range uint8(16) {
		t.Run(fmt.Sprintf("%X", k), func(t *testing.T) {
			m := NewMachine()
			m.KeyDown(k)
			for i, down := range m.keyPressed {
				if down != (uint8(i) == k) {
					t.Errorf("after KeyDown(%X), key %X is down = %v", k, i, down)
				}
			}
			m.KeyUp(k)
			if m.keyPressed != [16]bool{} {
				t.Errorf("after KeyUp(%X), keys still down: %v", k, m.keyPressed)
			}
		})
	}
}

// Keys are held down, not toggled: pressing twice is the same as pressing once,
// and releasing a key that isn't down does nothing.
func TestKeyDownUpAreNotToggles(t *testing.T) {
	m := NewMachine()
	m.KeyDown(3)
	m.KeyDown(3)
	if !m.keyPressed[3] {
		t.Errorf("key 3 not down after two KeyDown calls")
	}
	m.KeyUp(3)
	m.KeyUp(3)
	if m.keyPressed[3] {
		t.Errorf("key 3 still down after KeyUp")
	}
	m.KeyUp(7) // never pressed
	if m.keyPressed != [16]bool{} {
		t.Errorf("KeyUp of a key that wasn't down changed the keypad: %v", m.keyPressed)
	}
}

// A key number past 0xF is not a CHIP-8 key. It must be ignored, not panic and
// not wrap round onto a real key.
func TestKeyOutOfRangeIgnored(t *testing.T) {
	for _, k := range []uint8{16, 17, 0x1f, 0x20, 0x80, 0xff} {
		t.Run(fmt.Sprintf("%02x", k), func(t *testing.T) {
			m := NewMachine()
			if !noPanic(t, "KeyDown", func() { m.KeyDown(k) }) {
				return
			}
			if m.keyPressed != [16]bool{} {
				t.Errorf("KeyDown(%#x) pressed a key: %v", k, m.keyPressed)
			}
			m.KeyDown(k & 0x0f)
			if !noPanic(t, "KeyUp", func() { m.KeyUp(k) }) {
				return
			}
			if !m.keyPressed[k&0x0f] {
				t.Errorf("KeyUp(%#x) released key %X", k, k&0x0f)
			}
		})
	}
}

func TestKeysAreIndependent(t *testing.T) {
	m := NewMachine()
	m.KeyDown(1)
	m.KeyDown(6)
	m.KeyDown(0xf)
	m.KeyUp(6)
	for i, want := range [16]bool{1: true, 0xf: true} {
		if m.keyPressed[i] != want {
			t.Errorf("key %X down = %v, want %v", i, m.keyPressed[i], want)
		}
	}
}

// ---------------------------------------------------------------------------
// Ex9E / ExA1
// ---------------------------------------------------------------------------

// skipProgram is "LD V0, key; SKP/SKNP V0; LD V1, 1; LD V2, 2", so that whether
// the LD V1 was skipped shows in V1.
func skipProgram(key, lowByte byte) []byte {
	return []byte{
		0x60, key, // LD V0, key
		0xe0, lowByte, // SKP V0 (9E) / SKNP V0 (A1)
		0x61, 0x01, // LD V1, 1
		0x62, 0x02, // LD V2, 2
	}
}

func runSkipProgram(t *testing.T, key, lowByte byte, pressed ...uint8) *Machine {
	t.Helper()
	m := newMachineWithROM(t, skipProgram(key, lowByte))
	for _, k := range pressed {
		m.KeyDown(k)
	}
	step(t, m) // LD V0
	step(t, m) // the skip instruction
	return m
}

func TestSkipIfKeyPressed(t *testing.T) { // Ex9E
	for k := range uint8(16) {
		t.Run(fmt.Sprintf("key %X down", k), func(t *testing.T) {
			m := runSkipProgram(t, k, 0x9e, k)
			if m.pc != RomAddress+6 {
				t.Errorf("pc = %03x, want %03x (skipped)", m.pc, RomAddress+6)
			}
			step(t, m)
			if m.regV[1] != 0 || m.regV[2] != 2 {
				t.Errorf("V1, V2 = %d, %d, want 0, 2: skipped the wrong instruction", m.regV[1], m.regV[2])
			}
		})
		t.Run(fmt.Sprintf("key %X up", k), func(t *testing.T) {
			m := runSkipProgram(t, k, 0x9e)
			if m.pc != RomAddress+4 {
				t.Errorf("pc = %03x, want %03x (not skipped)", m.pc, RomAddress+4)
			}
			step(t, m)
			if m.regV[1] != 1 {
				t.Errorf("V1 = %d, want 1: the next instruction didn't run", m.regV[1])
			}
		})
	}
}

func TestSkipIfKeyNotPressed(t *testing.T) { // ExA1
	for k := range uint8(16) {
		t.Run(fmt.Sprintf("key %X up", k), func(t *testing.T) {
			m := runSkipProgram(t, k, 0xa1)
			if m.pc != RomAddress+6 {
				t.Errorf("pc = %03x, want %03x (skipped)", m.pc, RomAddress+6)
			}
			step(t, m)
			if m.regV[1] != 0 || m.regV[2] != 2 {
				t.Errorf("V1, V2 = %d, %d, want 0, 2: skipped the wrong instruction", m.regV[1], m.regV[2])
			}
		})
		t.Run(fmt.Sprintf("key %X down", k), func(t *testing.T) {
			m := runSkipProgram(t, k, 0xa1, k)
			if m.pc != RomAddress+4 {
				t.Errorf("pc = %03x, want %03x (not skipped)", m.pc, RomAddress+4)
			}
			step(t, m)
			if m.regV[1] != 1 {
				t.Errorf("V1 = %d, want 1: the next instruction didn't run", m.regV[1])
			}
		})
	}
}

// Only the key in Vx matters. Every other key being down must not make Ex9E skip,
// or stop ExA1 skipping.
func TestSkipOnlyLooksAtTheKeyInVx(t *testing.T) {
	var others []uint8
	for k := range uint8(16) {
		if k != 5 {
			others = append(others, k)
		}
	}
	if m := runSkipProgram(t, 5, 0x9e, others...); m.pc != RomAddress+4 {
		t.Errorf("Ex9E skipped with key 5 up and every other key down")
	}
	if m := runSkipProgram(t, 5, 0xa1, others...); m.pc != RomAddress+6 {
		t.Errorf("ExA1 didn't skip with key 5 up and every other key down")
	}
}

// Only the low nibble of Vx names the key, as with Fx29.
func TestSkipUsesLowNibbleOfVx(t *testing.T) {
	for _, vx := range []byte{0x15, 0x25, 0x85, 0xf5, 0xe5} {
		t.Run(fmt.Sprintf("V0=%02x", vx), func(t *testing.T) {
			if m := runSkipProgram(t, vx, 0x9e, 5); m.pc != RomAddress+6 {
				t.Errorf("Ex9E with V0=%02x didn't treat it as key 5", vx)
			}
			if m := runSkipProgram(t, vx, 0xa1, 5); m.pc != RomAddress+4 {
				t.Errorf("ExA1 with V0=%02x didn't treat it as key 5", vx)
			}
			// ...and the high nibble must not select another key
			if m := runSkipProgram(t, vx, 0x9e, vx>>4); vx>>4 != 5 && m.pc != RomAddress+4 {
				t.Errorf("Ex9E with V0=%02x looked at key %X", vx, vx>>4)
			}
		})
	}
}

func TestSkipUsesRegisterX(t *testing.T) {
	for x := range byte(16) {
		t.Run(fmt.Sprintf("V%X", x), func(t *testing.T) {
			m := newMachineWithROM(t, []byte{0xe0 | x, 0x9e})
			m.regV[x] = 9
			m.KeyDown(9)
			step(t, m)
			if m.pc != RomAddress+4 {
				t.Errorf("pc = %03x, want %03x", m.pc, RomAddress+4)
			}
		})
	}
}

// The skip instructions only read; they don't change registers, I, memory or the
// keys.
func TestSkipChangesNothingElse(t *testing.T) {
	for _, low := range []byte{0x9e, 0xa1} {
		t.Run(fmt.Sprintf("%02x", low), func(t *testing.T) {
			m := newMachineWithROM(t, []byte{0xe3, low})
			for i := range m.regV {
				m.regV[i] = uint8(i + 1)
			}
			m.regI = 0x345
			m.KeyDown(4)
			regs, i, mem, keys := m.regV, m.regI, m.mem, m.keyPressed
			step(t, m)
			if m.regV != regs || m.regI != i || m.mem != mem || m.keyPressed != keys {
				t.Errorf("a skip instruction changed registers, I, memory or the keys")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Fx0A
// ---------------------------------------------------------------------------

// stepWaiting runs the Fx0A at the start of the program and checks that it is
// still waiting: no error, the program counter back on the instruction, and the
// register untouched.
func stepWaiting(t *testing.T, m *Machine, reg byte, wantReg uint8) {
	t.Helper()
	step(t, m)
	if m.pc != RomAddress {
		t.Errorf("pc = %03x, want %03x: Fx0A should still be waiting", m.pc, RomAddress)
	}
	if m.regV[reg] != wantReg {
		t.Errorf("V%X = %02x while waiting, want %02x (unchanged)", reg, m.regV[reg], wantReg)
	}
}

// stepFinished runs the Fx0A and checks that it has completed with the key in Vx.
func stepFinished(t *testing.T, m *Machine, reg byte, wantKey uint8) {
	t.Helper()
	step(t, m)
	if m.pc != RomAddress+2 {
		t.Errorf("pc = %03x, want %03x: Fx0A should have finished", m.pc, RomAddress+2)
	}
	if m.regV[reg] != wantKey {
		t.Errorf("V%X = %02x, want %02x", reg, m.regV[reg], wantKey)
	}
}

func TestWaitKeyWaitsWhenNoKey(t *testing.T) {
	m := newMachineWithROM(t, []byte{0xf3, 0x0a})
	m.regV[3] = 0x77
	for range 5 {
		stepWaiting(t, m, 3, 0x77)
	}
}

// A press and a release give the key, and only on the release.
func TestWaitKeyCompletesOnRelease(t *testing.T) {
	for k := range uint8(16) {
		t.Run(fmt.Sprintf("%X", k), func(t *testing.T) {
			m := newMachineWithROM(t, []byte{0xf3, 0x0a})
			stepWaiting(t, m, 3, 0)
			m.KeyDown(k)
			stepWaiting(t, m, 3, 0) // pressed, not yet released
			stepWaiting(t, m, 3, 0)
			m.KeyUp(k)
			stepFinished(t, m, 3, k)
		})
	}
}

// A key that was already down when the instruction first ran counts: the
// instruction waits for it to be released and returns it.
func TestWaitKeyKeyAlreadyHeld(t *testing.T) {
	m := newMachineWithROM(t, []byte{0xf3, 0x0a})
	m.KeyDown(0xc)
	stepWaiting(t, m, 3, 0)
	stepWaiting(t, m, 3, 0)
	m.KeyUp(0xc)
	stepFinished(t, m, 3, 0xc)
}

// Releasing a key that was pressed before the instruction started, then pressing
// nothing, must not complete it with that key.
func TestWaitKeyNeedsAPress(t *testing.T) {
	m := newMachineWithROM(t, []byte{0xf3, 0x0a})
	m.KeyDown(2)
	m.KeyUp(2) // pressed and released before Fx0A ever ran
	for range 3 {
		stepWaiting(t, m, 3, 0)
	}
}

// Fx0A returns the key it first saw. Other keys coming and going while it waits
// don't change the answer, and don't complete it.
func TestWaitKeyLatchesFirstKey(t *testing.T) {
	m := newMachineWithROM(t, []byte{0xf3, 0x0a})
	m.KeyDown(4)
	stepWaiting(t, m, 3, 0) // latches key 4
	m.KeyDown(9)
	stepWaiting(t, m, 3, 0)
	m.KeyUp(9) // a different key being released isn't the end of the wait
	stepWaiting(t, m, 3, 0)
	m.KeyUp(4)
	stepFinished(t, m, 3, 4)
}

// With several keys down at once, which one it returns is not defined by CHIP-8,
// but it must be one of them, and the same one every time.
func TestWaitKeyWithSeveralKeysDown(t *testing.T) {
	var first uint8
	for attempt := range 5 {
		m := newMachineWithROM(t, []byte{0xf3, 0x0a})
		m.KeyDown(0xa)
		m.KeyDown(2)
		m.KeyDown(7)
		stepWaiting(t, m, 3, 0)
		m.KeyUp(0xa)
		m.KeyUp(2)
		m.KeyUp(7)
		step(t, m)
		got := m.regV[3]
		if got != 0xa && got != 2 && got != 7 {
			t.Fatalf("V3 = %X, want one of the keys that were down (2, 7 or A)", got)
		}
		if attempt == 0 {
			first = got
		} else if got != first {
			t.Errorf("attempt %d returned key %X, attempt 0 returned %X", attempt, got, first)
		}
	}
}

// When several keys are down, Fx0A returns the lowest-numbered one. CHIP-8 doesn't
// define this, so this test pins the choice this emulator made, to stop it
// changing by accident.
func TestWaitKeyPicksLowestKeyDown(t *testing.T) {
	for _, keys := range [][]uint8{
		{0, 1}, {1, 0}, {2, 9}, {9, 2}, {0xf, 3, 0xa}, {0xe, 0xf}, {0xf, 0xe}, {5, 6, 7},
	} {
		t.Run(fmt.Sprint(keys), func(t *testing.T) {
			want := keys[0]
			for _, k := range keys {
				want = min(want, k)
			}
			m := newMachineWithROM(t, []byte{0xf3, 0x0a})
			m.regV[3] = 0xee // so that 0 can't be a leftover
			for _, k := range keys {
				m.KeyDown(k)
			}
			stepWaiting(t, m, 3, 0xee)
			for _, k := range keys {
				m.KeyUp(k)
			}
			stepFinished(t, m, 3, want)
		})
	}
}

func TestWaitKeyUsesRegisterX(t *testing.T) {
	for x := range byte(16) {
		t.Run(fmt.Sprintf("V%X", x), func(t *testing.T) {
			m := newMachineWithROM(t, []byte{0xf0 | x, 0x0a})
			for i := range m.regV {
				m.regV[i] = 0x80 + uint8(i)
			}
			regs := m.regV
			m.KeyDown(6)
			stepWaiting(t, m, x, regs[x])
			m.KeyUp(6)
			stepFinished(t, m, x, 6)
			regs[x] = 6
			if m.regV != regs {
				t.Errorf("registers = %v, want only V%X changed: %v", m.regV, x, regs)
			}
		})
	}
}

// Once an Fx0A has finished, the next one starts from scratch: it waits for a new
// key instead of returning the old one straight away.
func TestWaitKeyTwiceInARow(t *testing.T) {
	m := newMachineWithROM(t, []byte{
		0xf0, 0x0a, // LD V0, K
		0xf1, 0x0a, // LD V1, K
	})
	m.KeyDown(3)
	step(t, m)
	m.KeyUp(3)
	step(t, m) // first one finishes with 3
	if m.regV[0] != 3 || m.pc != RomAddress+2 {
		t.Fatalf("first Fx0A: V0 = %X, pc = %03x, want 3, %03x", m.regV[0], m.pc, RomAddress+2)
	}
	for range 3 {
		step(t, m)
		if m.pc != RomAddress+2 {
			t.Fatalf("second Fx0A finished (pc = %03x) with no new key", m.pc)
		}
	}
	m.KeyDown(8)
	step(t, m)
	m.KeyUp(8)
	step(t, m)
	if m.regV[1] != 8 || m.pc != RomAddress+4 {
		t.Errorf("second Fx0A: V1 = %X, pc = %03x, want 8, %03x", m.regV[1], m.pc, RomAddress+4)
	}
	if m.regV[0] != 3 {
		t.Errorf("V0 = %X, want 3 still", m.regV[0])
	}
}

// The same key pressed again after a finished wait is a new wait's key.
func TestWaitKeySameKeyAgain(t *testing.T) {
	m := newMachineWithROM(t, []byte{0xf0, 0x0a, 0xf1, 0x0a})
	for i := range byte(2) {
		m.KeyDown(5)
		step(t, m)
		m.KeyUp(5)
		step(t, m)
		if m.regV[i] != 5 || m.pc != RomAddress+2*(uint16(i)+1) {
			t.Fatalf("wait %d: V%d = %X, pc = %03x", i, i, m.regV[i], m.pc)
		}
	}
}

// Timers are not paused while the machine waits for a key. (The keypad ROM tests
// for exactly this.)
func TestWaitKeyTimersKeepCounting(t *testing.T) {
	m := newMachineWithROM(t, []byte{0xf3, 0x0a})
	setTimers(m, 10, 10)
	for range 3 {
		stepWaiting(t, m, 3, 0)
		m.Tick()
	}
	wantTimers(t, m, 7, 7)
}

// Fx0A with no keys must not touch anything except the program counter staying
// put, however long it waits.
func TestWaitKeyChangesNothingWhileWaiting(t *testing.T) {
	m := newMachineWithROM(t, []byte{0xf3, 0x0a})
	for i := range m.regV {
		m.regV[i] = uint8(i + 1)
	}
	m.regI = 0x123
	regs, i, mem, sp := m.regV, m.regI, m.mem, m.sp
	for range 10 {
		step(t, m)
	}
	if m.regV != regs || m.regI != i || m.mem != mem || m.sp != sp {
		t.Errorf("waiting for a key changed registers, I, memory or sp")
	}
}

// Loading a new program abandons a wait that was in progress: the new program's
// Fx0A must not pick up the old one's key.
func TestLoadAbandonsKeyWait(t *testing.T) {
	m := newMachineWithROM(t, []byte{0xf3, 0x0a})
	m.KeyDown(6)
	stepWaiting(t, m, 3, 0) // latches key 6
	m.KeyUp(6)
	if err := m.Load(bytes.NewReader([]byte{0xf3, 0x0a})); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		stepWaiting(t, m, 3, 0)
	}
}

// Loading a program starts the machine afresh, keys included: a key that was down
// before the Load reads as up to the new program, until it is pressed again. A
// frontend that wants a held key to carry over has to send KeyDown again after
// loading.
func TestLoadReleasesHeldKeys(t *testing.T) {
	m := NewMachine()
	for k := range uint8(16) {
		m.KeyDown(k)
	}
	if err := m.Load(bytes.NewReader([]byte{
		0x60, 0x05, // LD V0, 5
		0xe0, 0x9e, // SKP V0
		0x61, 0x01, // LD V1, 1
		0xf2, 0x0a, // LD V2, K
	})); err != nil {
		t.Fatal(err)
	}
	if m.keyPressed != [16]bool{} {
		t.Errorf("keys still down after Load: %v", m.keyPressed)
	}
	step(t, m) // LD V0
	step(t, m) // SKP V0: key 5 was down before Load, so it must not skip
	if m.pc != RomAddress+4 {
		t.Errorf("SKP skipped after Load: pc = %03x, want %03x", m.pc, RomAddress+4)
	}
	step(t, m) // LD V1
	for range 3 {
		step(t, m) // LD V2, K: no key is down, so it must still be waiting
		if m.pc != RomAddress+6 {
			t.Fatalf("Fx0A finished with no key pressed since Load: pc = %03x", m.pc)
		}
	}
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

// A frontend presses keys from its own goroutine while the machine runs. Under
// -race this catches key state that isn't locked.
func TestKeysConcurrentWithStep(t *testing.T) {
	m := newMachineWithROM(t, []byte{
		0xf0, 0x0a, // 200: LD V0, K
		0x61, 0x01, // 202: LD V1, 1
		0xe0, 0x9e, // 204: SKP V0
		0x12, 0x00, // 206: JP 200
		0x12, 0x08, // 208: JP 208 (halt)
	})
	var stop atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			m.KeyDown(7)
			m.KeyUp(7)
			m.KeyDown(0x1f) // out of range, must be ignored
			m.KeyUp(0x1f)
		}
	}()
	deadline := time.Now().Add(100 * time.Millisecond)
	steps := 0
	for time.Now().Before(deadline) {
		if err := m.Step(); err != nil {
			stop.Store(true)
			wg.Wait()
			t.Fatalf("Step: %s", err)
		}
		steps++
	}
	stop.Store(true)
	wg.Wait()
	if steps == 0 {
		t.Errorf("no steps ran")
	}
}

// ---------------------------------------------------------------------------
// Test-suite ROMs
//
// Like the ROM tests in machine_test.go, the expected screens here are the
// published screenshots from github.com/Timendus/chip8-test-suite (pictures/),
// converted to pixels with each light pixel as '#'. They were not produced by this
// emulator.
// ---------------------------------------------------------------------------

// The keypad ROM can skip its menu: the value 1-3 at address 0x1FF selects a test.
const keypadROMSelect = 0x1ff

// stepsPerTick is roughly how many instructions a real CHIP-8 runs per 60Hz timer
// tick (about 700 instructions a second). The ROM tests tick at this rate so that
// timer-driven animations and waits behave as they would on a real machine.
const stepsPerTick = 12

// keypadROM returns a machine running the keypad test with the given sub-test
// selected (1 = Ex9E, 2 = ExA1, 3 = Fx0A, 0 = the menu).
func keypadROM(t *testing.T, test byte) *Machine {
	t.Helper()
	m := newMachineWithROMFile(t, "testdata/6-keypad.ch8")
	m.mem[keypadROMSelect] = test
	return m
}

// runTicking runs n instructions, ticking the timers at a realistic rate.
func runTicking(t *testing.T, m *Machine, n int) {
	t.Helper()
	for i := range n {
		if i%stepsPerTick == 0 {
			m.Tick()
		}
		step(t, m)
	}
}

// runNoTicks runs n instructions without ticking the timers. For screens that don't
// depend on the timers it keeps the result independent of the tick rate.
func runNoTicks(t *testing.T, m *Machine, n int) {
	t.Helper()
	for range n {
		step(t, m)
	}
}

func wantScreen(t *testing.T, m *Machine, golden string) {
	t.Helper()
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("reading expected output: %s", err)
	}
	if got := renderDisplay(m.disp); got != string(want) {
		t.Errorf("display mismatch with %s\n--- got ---\n%s--- want ---\n%s", golden, got, want)
	}
}

// With no input the ROM shows its menu.
func TestKeypadROMMenu(t *testing.T) {
	m := keypadROM(t, 0)
	runNoTicks(t, m, 5000)
	wantScreen(t, m, "testdata/6-keypad-menu.txt")
}

// The Ex9E test lights up the value of each key that is pressed. The published
// screen is for keys 1 and 6.
func TestKeypadROMSkipIfPressed(t *testing.T) {
	m := keypadROM(t, 1)
	m.KeyDown(1)
	m.KeyDown(6)
	runNoTicks(t, m, 5000)
	wantScreen(t, m, "testdata/6-keypad-down.txt")
}

// The ExA1 test lights up the value of each key that is NOT pressed.
func TestKeypadROMSkipIfNotPressed(t *testing.T) {
	m := keypadROM(t, 2)
	m.KeyDown(1)
	m.KeyDown(6)
	runNoTicks(t, m, 5000)
	wantScreen(t, m, "testdata/6-keypad-up.txt")
}

// Releasing keys must be seen too: with the keys let go, the Ex9E screen goes
// blank of key values, and the ExA1 screen has all of them.
func TestKeypadROMSeesKeysReleased(t *testing.T) {
	for _, test := range []byte{1, 2} {
		t.Run(fmt.Sprint(test), func(t *testing.T) {
			m := keypadROM(t, test)
			m.KeyDown(1)
			m.KeyDown(6)
			runNoTicks(t, m, 5000)
			held := renderDisplay(m.disp)
			m.KeyUp(1)
			m.KeyUp(6)
			runNoTicks(t, m, 5000)
			if renderDisplay(m.disp) == held {
				t.Errorf("screen didn't change when the keys were released")
			}
			m.KeyDown(1)
			m.KeyDown(6)
			runNoTicks(t, m, 5000)
			if renderDisplay(m.disp) != held {
				t.Errorf("screen with keys 1 and 6 pressed again differs from the first time")
			}
		})
	}
}

// The Fx0A test asks for a key, and then checks that the machine really waited
// for it (with the timers still running) and that it waited for the key to be
// released. The published screen is the all-good one.
func TestKeypadROMWaitForKey(t *testing.T) {
	for _, k := range []uint8{0, 1, 5, 0xa, 0xf} {
		t.Run(fmt.Sprintf("key %X", k), func(t *testing.T) {
			m := keypadROM(t, 3)
			runTicking(t, m, 3000) // sits at the prompt, timers running
			prompt := renderDisplay(m.disp)
			m.KeyDown(k)
			runTicking(t, m, 3000)
			if got := renderDisplay(m.disp); got != prompt {
				t.Errorf("the screen changed while the key was still held down; Fx0A should wait for the release\n%s", got)
			}
			m.KeyUp(k)
			runTicking(t, m, 3000)
			wantScreen(t, m, "testdata/6-keypad-getkey.txt")
		})
	}
}

// ---------------------------------------------------------------------------
// Beep
// ---------------------------------------------------------------------------

// The beep ROM sounds SOS in morse code (three short, three long, three short),
// and shows a speaker icon on the display during each beep. The sound timer is the
// only way this emulator shows that a beep is on, so the test follows it, and checks
// the icon against it.
//
// testdata/7-beep.txt is the published screen, pictures/beep.png, with the icon
// showing. It was not produced by this emulator.
func TestBeepROMSignalsSOS(t *testing.T) {
	m := newMachineWithROMFile(t, "testdata/7-beep.ch8")
	icon, err := os.ReadFile("testdata/7-beep.txt")
	if err != nil {
		t.Fatal(err)
	}

	var beeps []int // length of each beep in ticks
	onTicks := 0
	disagree, longestDisagree := 0, 0
	for i := range 6000 {
		if i%stepsPerTick == 0 {
			m.Tick()
			if m.soundTimer > 0 {
				onTicks++
			} else if onTicks > 0 {
				beeps = append(beeps, onTicks)
				onTicks = 0
			}
		}
		step(t, m)
		// The icon is drawn and the timer set a few instructions apart, so allow
		// them to disagree briefly, but not for long.
		if (renderDisplay(m.disp) == string(icon)) != (m.soundTimer > 0) {
			disagree++
			longestDisagree = max(longestDisagree, disagree)
		} else {
			disagree = 0
		}
	}
	if len(beeps) < 9 {
		t.Fatalf("saw %d beeps, want at least 9 (S O S): %v", len(beeps), beeps)
	}
	const short, long, slack = 10, 30, 2
	for i, want := range []int{short, short, short, long, long, long, short, short, short} {
		if d := beeps[i] - want; d < -slack || d > slack {
			t.Errorf("beep %d lasted %d ticks, want about %d (all: %v)", i+1, beeps[i], want, beeps[:9])
		}
	}
	if longestDisagree > 30 {
		t.Errorf("the speaker icon and the sound timer disagreed for %d instructions in a row", longestDisagree)
	}
}

// Pressing B takes manual control of the buzzer: it sounds while B is held.
func TestBeepROMManualBuzzer(t *testing.T) {
	m := newMachineWithROMFile(t, "testdata/7-beep.ch8")
	icon, err := os.ReadFile("testdata/7-beep.txt")
	if err != nil {
		t.Fatal(err)
	}
	// Let the automatic SOS be running first, then take over.
	runTicking(t, m, 500)
	m.KeyDown(0xb)
	runTicking(t, m, 2000)
	if _, s := timers(m); s == 0 {
		t.Errorf("sound timer is 0 while B is held down")
	}
	if got := renderDisplay(m.disp); got != string(icon) {
		t.Errorf("speaker icon not shown while B is held down\n%s", got)
	}
	m.KeyUp(0xb)
	runTicking(t, m, 2000)
	if _, s := timers(m); s != 0 {
		t.Errorf("sound timer = %d after B was released, want 0", s)
	}
}
