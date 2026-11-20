package gremul8

import (
	"bytes"
	"os"
	"testing"
)

// ---------------------------------------------------------------------------
// Test ROMs
//
// The expected screens in testdata/*.txt were not produced by this emulator.
// They came from a throwaway reference interpreter written separately against
// the original CHIP-8 behavior (it is not kept in this repository), and were
// later confirmed by a second, independently written interpreter. Do not
// regenerate them from this code.
// ---------------------------------------------------------------------------

// testROM runs a ROM until it halts and compares the final screen with the
// expected one. wantSteps is the number of instructions that run before the
// halting jump.
func testROM(t *testing.T, rom, golden string, wantSteps int) {
	t.Helper()
	m := newMachineWithROMFile(t, rom)
	if got := runToHalt(t, m, 1000); got != wantSteps {
		t.Errorf("ROM halted after %d instructions, want %d", got, wantSteps)
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("reading expected output: %s", err)
	}
	if got := renderDisplay(m.disp); got != string(want) {
		t.Errorf("display mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestIBMLogo(t *testing.T) {
	testROM(t, "testdata/2-ibm-logo.ch8", "testdata/2-ibm-logo.txt", 20)
}

func TestCHIP8Logo(t *testing.T) {
	testROM(t, "testdata/1-chip8-logo.ch8", "testdata/1-chip8-logo.txt", 39)
}

// Corax+ exercises 3XNN, 4XNN, 5XY0, 9XY0, 7XNN, all of 8XY*, FX1E, FX33,
// FX55 and FX65, and draws a checkmark (pass) or cross (fail) next to each test.
//
// testdata/3-corax+.txt is the published all-checkmarks screen, pictures/corax+.png
// in github.com/Timendus/chip8-test-suite, converted to pixels. It was not
// produced by this emulator. The 306-step count is this emulator's own, and is
// only a regression guard.
func TestCorax(t *testing.T) {
	testROM(t, "testdata/3-corax+.ch8", "testdata/3-corax+.txt", 306)
}

// ---------------------------------------------------------------------------
// Drawing
// ---------------------------------------------------------------------------

// DXYN sets VF to 1 when it turns off a lit pixel, and must set it back to 0 on
// a draw that doesn't. The IBM logo ROM never overlaps sprites, so it can't
// exercise either case.
func TestDrawVF(t *testing.T) {
	m := newMachineWithROM(t, []byte{
		0xa2, 0x0a, // 0x200: LD I, 0x20a
		0xd0, 0x11, // 0x202: DRW V0, V1, 1
		0xd0, 0x11, // 0x204: DRW V0, V1, 1 (same place, erases the sprite)
		0x60, 0x08, // 0x206: LD V0, 8
		0xd0, 0x11, // 0x208: DRW V0, V1, 1 (somewhere new, no collision)
		0xc0, // 0x20a: sprite data, two pixels
	})
	row := func() string { return rowString(m.disp, 0)[:12] }

	step(t, m) // LD I
	step(t, m) // first draw
	if m.regV[0xf] != 0 {
		t.Errorf("after first draw: VF = %d, want 0", m.regV[0xf])
	}
	if got, want := row(), "##.........."; got != want {
		t.Errorf("after first draw: row 0 = %q, want %q", got, want)
	}

	step(t, m) // second draw, same place
	if m.regV[0xf] != 1 {
		t.Errorf("after overlapping draw: VF = %d, want 1", m.regV[0xf])
	}
	if got, want := row(), "............"; got != want {
		t.Errorf("after overlapping draw: row 0 = %q, want %q", got, want)
	}

	step(t, m) // LD V0, 8
	step(t, m) // third draw, no collision
	if m.regV[0xf] != 0 {
		t.Errorf("after non-overlapping draw: VF = %d, want 0 (should reset)", m.regV[0xf])
	}
	if got, want := row(), "........##.."; got != want {
		t.Errorf("after non-overlapping draw: row 0 = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Arithmetic and flow
// ---------------------------------------------------------------------------

// 7XNN adds without carry: it wraps at 256 and must not touch VF.
func TestAddImmediateWraps(t *testing.T) {
	m := newMachineWithROM(t, []byte{
		0x6f, 0x07, // LD VF, 7
		0x60, 0xff, // LD V0, 255
		0x70, 0x02, // ADD V0, 2
	})
	for range 3 {
		step(t, m)
	}
	if m.regV[0] != 1 {
		t.Errorf("V0 = %d, want 1 (255 + 2 wraps to 1)", m.regV[0])
	}
	if m.regV[0xf] != 7 {
		t.Errorf("VF = %d, want 7 (7XNN must not change VF)", m.regV[0xf])
	}
}

// A jump to the instruction's own address is how CHIP-8 programs halt. It must
// stay put, however many times it runs.
func TestJumpToSelf(t *testing.T) {
	m := newMachineWithROM(t, []byte{0x12, 0x00}) // JP 0x200
	for i := range 3 {
		step(t, m)
		if m.pc != RomAddress {
			t.Fatalf("after %d jumps to self: pc = %04x, want %04x", i+1, m.pc, RomAddress)
		}
	}
}

// A call must jump to the subroutine, and the return must resume at the
// instruction immediately after the call (not skip it, and not repeat the call).
// A return must also take the address off the stack.
func TestCallReturn(t *testing.T) {
	m := newMachineWithROM(t, []byte{
		0x22, 0x06, // 0x200: CALL 0x206
		0x60, 0x02, // 0x202: LD V0, 2 (must run after the return)
		0x00, 0xee, // 0x204: RET (nothing left on the stack: must be an error)
		0x61, 0x01, // 0x206: LD V1, 1
		0x00, 0xee, // 0x208: RET
	})

	step(t, m) // CALL
	if m.pc != 0x206 {
		t.Errorf("after CALL: pc = %04x, want 0206", m.pc)
	}

	step(t, m) // LD V1, 1
	step(t, m) // RET
	if m.pc != 0x202 {
		t.Errorf("after RET: pc = %04x, want 0202 (the instruction after the CALL)", m.pc)
	}

	step(t, m) // LD V0, 2
	if m.regV[0] != 2 {
		t.Errorf("V0 = %d, want 2: the instruction after the CALL did not run", m.regV[0])
	}
	if m.regV[1] != 1 {
		t.Errorf("V1 = %d, want 1: the subroutine did not run", m.regV[1])
	}

	// The return address must have been popped, so this has nothing to return to.
	if ok, err := stepNoPanic(t, m); ok {
		wantErrContaining(t, err, "empty stack")
	}
}

// Nested calls must return to the right place at each level.
func TestCallReturnNested(t *testing.T) {
	m := newMachineWithROM(t, []byte{
		0x22, 0x06, // 0x200: CALL 0x206 (outer)
		0x60, 0x01, // 0x202: LD V0, 1 (after the outer call returns)
		0x00, 0xee, // 0x204: RET (nothing left on the stack: must be an error)
		0x22, 0x0c, // 0x206: outer: CALL 0x20c (inner)
		0x61, 0x02, // 0x208: LD V1, 2 (after the inner call returns)
		0x00, 0xee, // 0x20a: RET (from outer)
		0x62, 0x03, // 0x20c: inner: LD V2, 3
		0x00, 0xee, // 0x20e: RET (from inner)
	})

	step(t, m) // CALL outer
	step(t, m) // CALL inner
	if m.pc != 0x20c {
		t.Fatalf("after nested CALL: pc = %04x, want 020c", m.pc)
	}

	step(t, m) // LD V2, 3
	step(t, m) // RET from inner
	if m.pc != 0x208 {
		t.Errorf("after inner RET: pc = %04x, want 0208 (the instruction after the inner CALL)", m.pc)
	}

	step(t, m) // LD V1, 2
	step(t, m) // RET from outer
	if m.pc != 0x202 {
		t.Errorf("after outer RET: pc = %04x, want 0202 (the instruction after the outer CALL)", m.pc)
	}

	step(t, m) // LD V0, 1
	if m.regV[0] != 1 || m.regV[1] != 2 || m.regV[2] != 3 {
		t.Errorf("V0,V1,V2 = %d,%d,%d, want 1,2,3", m.regV[0], m.regV[1], m.regV[2])
	}

	// Both return addresses must have been popped.
	if ok, err := stepNoPanic(t, m); ok {
		wantErrContaining(t, err, "empty stack")
	}
}

// RET with nothing on the stack must be an error, not a panic.
func TestReturnEmptyStack(t *testing.T) {
	m := newMachineWithROM(t, []byte{0x00, 0xee})
	if ok, err := stepNoPanic(t, m); ok {
		wantErrContaining(t, err, "empty stack")
	}
}

// Calling more deeply than the stack allows must be an error, not a panic or a
// silent overwrite of memory.
func TestCallStackOverflow(t *testing.T) {
	// The original COSMAC VIP interpreter allowed 12 levels of subroutines, and
	// most others allow at least 16, so a much smaller limit would break
	// real programs. This also keeps the test below from passing vacuously with
	// a limit that is set too low.
	const minDepth = 12
	if StackMaxDepth < minDepth {
		t.Fatalf("StackMaxDepth = %d, want at least %d", StackMaxDepth, minDepth)
	}

	m := newMachineWithROM(t, []byte{0x22, 0x00}) // CALL 0x200 (calls itself)
	for i := range StackMaxDepth {
		ok, err := stepNoPanic(t, m)
		if !ok {
			return
		}
		if err != nil {
			t.Fatalf("call %d of %d: unexpected error: %s", i+1, StackMaxDepth, err)
		}
	}
	if ok, err := stepNoPanic(t, m); ok {
		wantErrContaining(t, err, "stack depth")
	}
	if !fontIntact(m) {
		t.Errorf("built-in font was overwritten by the stack overflow")
	}
}

// ---------------------------------------------------------------------------
// Program counter and loading
// ---------------------------------------------------------------------------

// An instruction occupies two bytes, so the last valid one starts at 0xFFE. A
// program counter that has run off the end must be an error, not a panic.
func TestProgramCounterBounds(t *testing.T) {
	t.Run("last valid instruction", func(t *testing.T) {
		m := newMachineWithROM(t, []byte{0x1f, 0xfe}) // JP 0xffe
		m.mem[0xffe], m.mem[0xfff] = 0x60, 0x07       // LD V0, 7
		step(t, m)                                    // JP
		if ok, err := stepNoPanic(t, m); ok && err != nil {
			t.Fatalf("instruction at 0xffe: unexpected error: %s", err)
		}
		if m.regV[0] != 7 {
			t.Errorf("V0 = %d, want 7: the instruction at 0xffe did not run", m.regV[0])
		}
		// pc is now past the end of memory
		if ok, err := stepNoPanic(t, m); ok {
			wantErrContaining(t, err, "end of memory")
		}
	})

	t.Run("instruction split across the end", func(t *testing.T) {
		m := newMachineWithROM(t, []byte{0x1f, 0xff}) // JP 0xfff
		step(t, m)
		if ok, err := stepNoPanic(t, m); ok {
			wantErrContaining(t, err, "end of memory")
		}
	})
}

// A program that fits in memory loads in full, and one that doesn't is an error
// and not silently truncated.
func TestLoadSizeLimit(t *testing.T) {
	const maxSize = MemSize - RomAddress

	rom := bytes.Repeat([]byte{0xaa}, maxSize)
	rom[0], rom[maxSize-1] = 0x11, 0x22 // mark the first and last bytes

	m := NewMachine()
	if err := m.Load(bytes.NewReader(rom)); err != nil {
		t.Fatalf("loading a %d byte program (the maximum): unexpected error: %s", maxSize, err)
	}
	if m.mem[RomAddress] != 0x11 || m.mem[MemSize-1] != 0x22 {
		t.Errorf("first and last program bytes = %02x, %02x, want 11, 22: the program was not copied in full",
			m.mem[RomAddress], m.mem[MemSize-1])
	}

	err := m.Load(bytes.NewReader(make([]byte, maxSize+1)))
	wantErrContaining(t, err, "exceeds")
}

// Loading a program into a machine that has already run something must give
// the same result as loading it into a new machine.
func TestLoadResetsMachine(t *testing.T) {
	m := newMachineWithROM(t, []byte{
		0x60, 0x05, // 0x200: LD V0, 5
		0x61, 0x07, // 0x202: LD V1, 7
		0xa0, 0x50, // 0x204: LD I, 0x050 (the font)
		0xd0, 0x05, // 0x206: DRW V0, V0, 5 (draw a glyph at 5,5)
		0x6f, 0x09, // 0x208: LD VF, 9
		0x22, 0x0e, // 0x20a: CALL 0x20e
		0x12, 0x0c, // 0x20c: JP 0x20c
		0x12, 0x0e, // 0x20e: JP 0x20e (never returns, leaves the call on the stack)
	})
	for range 6 {
		step(t, m)
	}
	// Make sure the machine is actually in a used state, or the checks below
	// would pass without testing anything.
	if m.regV[0] != 5 || m.regV[1] != 7 || m.regV[0xf] != 9 || m.regI == 0 ||
		litPixels(m.disp) == 0 || m.pc != 0x20e {
		t.Fatalf("setup: machine not in the expected used state (V0=%d V1=%d VF=%d I=%04x lit=%d pc=%04x)",
			m.regV[0], m.regV[1], m.regV[0xf], m.regI, litPixels(m.disp), m.pc)
	}

	if err := m.Load(bytes.NewReader([]byte{
		0x70, 0x01, // 0x200: ADD V0, 1
		0x00, 0xee, // 0x202: RET
	})); err != nil {
		t.Fatalf("reloading: %s", err)
	}

	if m.pc != RomAddress {
		t.Errorf("pc = %04x, want %04x", m.pc, RomAddress)
	}
	if m.regV != ([16]uint8{}) {
		t.Errorf("registers = %v, want all zero", m.regV)
	}
	if m.regI != 0 {
		t.Errorf("I = %04x, want 0", m.regI)
	}
	if n := litPixels(m.disp); n != 0 {
		t.Errorf("%d pixels still lit, want a clear display", n)
	}
	if m.mem[0x208] != 0 {
		t.Errorf("mem[0x208] = %02x, want 0: the old program was not cleared", m.mem[0x208])
	}
	if !fontIntact(m) {
		t.Errorf("built-in font missing after reload")
	}

	step(t, m) // ADD V0, 1
	if m.regV[0] != 1 {
		t.Errorf("V0 = %d, want 1: registers were not reset", m.regV[0])
	}
	// The old call is still on the stack if the stack was not reset.
	if ok, err := stepNoPanic(t, m); ok { // RET
		wantErrContaining(t, err, "empty stack")
	}
}
