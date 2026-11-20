package gremul8

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// Shared helpers for the tests in this package.

// rowString renders one row of the display as a string of '#' and '.', for
// readable comparisons in test failures.
func rowString(d Display, y uint8) string {
	w, _ := d.Size()
	out := make([]byte, w)
	for x := range w {
		if d.IsSet(x, y) {
			out[x] = '#'
		} else {
			out[x] = '.'
		}
	}
	return string(out)
}

// renderDisplay returns the display as text, one line per row, with '#' for
// lit pixels and '.' for unlit ones.
func renderDisplay(d Display) string {
	_, h := d.Size()
	var sb strings.Builder
	for y := range h {
		sb.WriteString(rowString(d, y))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// litPixels returns the number of lit pixels on the display.
func litPixels(d Display) int {
	w, h := d.Size()
	n := 0
	for y := range h {
		for x := range w {
			if d.IsSet(x, y) {
				n++
			}
		}
	}
	return n
}

// newMachineWithROM returns a machine with the given program loaded at the ROM
// address.
func newMachineWithROM(t *testing.T, rom []byte) *Machine {
	t.Helper()
	m := NewMachine()
	if err := m.Load(bytes.NewReader(rom)); err != nil {
		t.Fatalf("loading program: %s", err)
	}
	return m
}

// newMachineWithROMFile is like newMachineWithROM, but reads the program from a
// file.
func newMachineWithROMFile(t *testing.T, path string) *Machine {
	t.Helper()
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ROM: %s", err)
	}
	return newMachineWithROM(t, rom)
}

// step runs one instruction and fails the test on error.
func step(t *testing.T, m *Machine) {
	t.Helper()
	if err := m.Step(); err != nil {
		t.Fatalf("step (pc=%04x): %s", m.pc, err)
	}
}

// runToHalt runs instructions until one leaves the program counter where it
// was, and returns the number of instructions that ran before that one. On
// CHIP-8 that is how a program halts: with a jump to its own address. (Any
// other instruction that happens to leave pc unchanged, such as a call or return
// to its own address, would also stop it.)
func runToHalt(t *testing.T, m *Machine, maxSteps int) int {
	t.Helper()
	for n := range maxSteps {
		before := m.pc
		step(t, m)
		if m.pc == before {
			return n
		}
	}
	t.Fatalf("program did not halt within %d instructions (pc=%04x)", maxSteps, m.pc)
	return 0
}

// noPanic runs fn and reports a test failure, instead of crashing the whole test
// binary, if it panics. It returns false if fn panicked.
func noPanic(t *testing.T, what string, fn func()) (ok bool) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s panicked: %v", what, r)
			ok = false
		}
	}()
	fn()
	return true
}

// stepNoPanic runs one instruction and returns its error. A panic is reported as
// a test failure and returned as ok == false.
func stepNoPanic(t *testing.T, m *Machine) (ok bool, err error) {
	t.Helper()
	ok = noPanic(t, "Step", func() { err = m.Step() })
	return ok, err
}

// wantErrContaining fails the test unless err is non-nil and its message
// contains substr, so that a test cannot pass because of an unrelated error.
func wantErrContaining(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Errorf("expected an error containing %q, got nil", substr)
		return
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error = %q, want it to contain %q", err, substr)
	}
}

// fontIntact reports whether the built-in font is still in memory unmodified.
func fontIntact(m *Machine) bool {
	for i, ch := range BuiltinFont {
		for j, b := range ch {
			if m.mem[BuiltinFontAddress+i*BuiltinFontWidth+j] != b {
				return false
			}
		}
	}
	return true
}
