package gremul8

import (
	"bytes"
	"fmt"
	"math/bits"
	"math/rand/v2"
	"testing"
)

// Tests for individual opcodes, run in isolation rather than through a ROM.

func TestClearScreen(t *testing.T) {
	// LD I, font "0"; DRW V0, V0, 5; CLS
	m := newMachineWithROM(t, []byte{0xa0, 0x50, 0xd0, 0x05, 0x00, 0xe0})
	step(t, m)
	step(t, m)
	if litPixels(m.disp) == 0 {
		t.Fatalf("nothing drawn before the clear; test is not checking anything")
	}
	step(t, m)
	if n := litPixels(m.disp); n != 0 {
		t.Errorf("%d pixels lit after 00E0, want 0\n%s", n, renderDisplay(m.disp))
	}
}

// The standard CHIP-8 font, written out independently of font.go.
var wantFont = [16][5]byte{
	{0xf0, 0x90, 0x90, 0x90, 0xf0}, // 0
	{0x20, 0x60, 0x20, 0x20, 0x70}, // 1
	{0xf0, 0x10, 0xf0, 0x80, 0xf0}, // 2
	{0xf0, 0x10, 0xf0, 0x10, 0xf0}, // 3
	{0x90, 0x90, 0xf0, 0x10, 0x10}, // 4
	{0xf0, 0x80, 0xf0, 0x10, 0xf0}, // 5
	{0xf0, 0x80, 0xf0, 0x90, 0xf0}, // 6
	{0xf0, 0x10, 0x20, 0x40, 0x40}, // 7
	{0xf0, 0x90, 0xf0, 0x90, 0xf0}, // 8
	{0xf0, 0x90, 0xf0, 0x10, 0xf0}, // 9
	{0xf0, 0x90, 0xf0, 0x90, 0x90}, // A
	{0xe0, 0x90, 0xe0, 0x90, 0xe0}, // B
	{0xf0, 0x80, 0x80, 0x80, 0xf0}, // C
	{0xe0, 0x90, 0x90, 0x90, 0xe0}, // D
	{0xf0, 0x80, 0xf0, 0x80, 0xf0}, // E
	{0xf0, 0x80, 0xf0, 0x80, 0x80}, // F
}

// Drawing each glyph straight from the font table checks the font address, the
// glyph contents and the sprite bit order all at once, against literals that do
// not come from font.go.
func TestFontGlyphs(t *testing.T) {
	for i, glyph := range wantFont {
		t.Run(fmt.Sprintf("%X", i), func(t *testing.T) {
			addr := 0x050 + i*5
			// LD I, addr; DRW V0, V0, 5
			m := newMachineWithROM(t, []byte{
				0xa0 | byte(addr>>8), byte(addr), 0xd0, 0x05,
			})
			step(t, m)
			step(t, m)
			wantLit := 0
			for y, row := range glyph {
				wantLit += bits.OnesCount8(row)
				want := make([]byte, 8)
				for x := range want {
					want[x] = '.'
					if row&(0x80>>x) != 0 {
						want[x] = '#'
					}
				}
				if got := rowString(m.disp, uint8(y))[:8]; got != string(want) {
					t.Errorf("row %d = %q, want %q", y, got, want)
				}
			}
			if got := rowString(m.disp, 5)[:8]; got != "........" {
				t.Errorf("row 5 = %q, want blank", got)
			}
			// Nothing else may be lit anywhere on the display
			if n := litPixels(m.disp); n != wantLit {
				t.Errorf("%d pixels lit, want %d", n, wantLit)
			}
		})
	}
}

func TestUnknownOpcodes(t *testing.T) {
	for _, op := range []uint16{
		0x0000, 0x0123, 0x00e1, 0x00ef, // 0NNN other than 00E0/00EE
		// 8XYn values that are not CHIP-8 instructions (n is 8-D or F)
		0x8008, 0x8019, 0x802a, 0x803b, 0x804c, 0x805d, 0x806f,
		// FXnn values that are not instructions in CHIP-8, SCHIP or XO-CHIP, so
		// they should stay unimplemented. (Avoid the real ones: 07, 0A, 15, 18,
		// 1E, 29, 33, 55, 65; SCHIP's 30, 75, 85; XO-CHIP's 00 and 01 (F000,
		// FN01), 02 (F002), 3A (FX3A), 75 and 85.)
		0xf0fe, 0xf0ff, 0xf1fe, 0xf132, 0xf256, 0xf3a5, 0xf464, 0xfffe,
	} {
		t.Run(fmt.Sprintf("%04x", op), func(t *testing.T) {
			m := newMachineWithROM(t, []byte{byte(op >> 8), byte(op)})
			ok, err := stepNoPanic(t, m)
			if !ok {
				return
			}
			wantErrContaining(t, err, fmt.Sprintf("%04x", op))
		})
	}
}

func TestSimpleOpcodes(t *testing.T) {
	tests := []struct {
		name  string
		rom   []byte
		check func(t *testing.T, m *Machine)
	}{
		{"6XNN loads full byte", []byte{0x63, 0xff}, func(t *testing.T, m *Machine) {
			if m.regV[3] != 0xff {
				t.Errorf("V3 = %02x, want ff", m.regV[3])
			}
		}},
		{"6XNN into VF", []byte{0x6f, 0x42}, func(t *testing.T, m *Machine) {
			if m.regV[0xf] != 0x42 {
				t.Errorf("VF = %02x, want 42", m.regV[0xf])
			}
		}},
		{"6XNN touches only X", []byte{0x65, 0x01}, func(t *testing.T, m *Machine) {
			for i, v := range m.regV {
				if i != 5 && v != 0 {
					t.Errorf("V%x = %02x, want 0", i, v)
				}
			}
		}},
		{"ANNN loads 12-bit address", []byte{0xaf, 0xff}, func(t *testing.T, m *Machine) {
			if m.regI != 0xfff {
				t.Errorf("I = %03x, want fff", m.regI)
			}
		}},
		{"ANNN low address", []byte{0xa0, 0x01}, func(t *testing.T, m *Machine) {
			if m.regI != 0x001 {
				t.Errorf("I = %03x, want 001", m.regI)
			}
		}},
		{"1NNN jumps to 12-bit address", []byte{0x1f, 0xff}, func(t *testing.T, m *Machine) {
			if m.pc != 0xfff {
				t.Errorf("pc = %03x, want fff", m.pc)
			}
		}},
		{"1NNN jumps low", []byte{0x10, 0x00}, func(t *testing.T, m *Machine) {
			if m.pc != 0x000 {
				t.Errorf("pc = %03x, want 000", m.pc)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMachineWithROM(t, tt.rom)
			step(t, m)
			tt.check(t, m)
		})
	}
}

// 6XNN must not affect I, and ANNN must not affect the registers.
func TestSimpleOpcodesIndependent(t *testing.T) {
	m := newMachineWithROM(t, []byte{0xa1, 0x23, 0x60, 0x45})
	step(t, m)
	step(t, m)
	if m.regI != 0x123 || m.regV[0] != 0x45 {
		t.Errorf("I = %03x, V0 = %02x; want 123 and 45", m.regI, m.regV[0])
	}
}

// A zero-height sprite draws nothing, but still reports "no collision" in VF.
func TestDrawZeroHeight(t *testing.T) {
	// LD VF, 1; DRW V0, V0, 0
	m := newMachineWithROM(t, []byte{0x6f, 0x01, 0xd0, 0x00})
	step(t, m)
	step(t, m)
	if n := litPixels(m.disp); n != 0 {
		t.Errorf("%d pixels lit, want 0", n)
	}
	if m.regV[0xf] != 0 {
		t.Errorf("VF = %d, want 0", m.regV[0xf])
	}
}

// DXYN reads N bytes starting at I, so the last legal sprite ends exactly at the
// end of memory, and anything beyond that must be an error rather than a panic.
func TestDrawSpriteMemoryBounds(t *testing.T) {
	tests := []struct {
		name    string
		i       uint16
		n       byte
		wantErr bool
		wantLit int // pixels lit when there is no error
	}{
		{"last byte of memory", 0xfff, 1, false, 1},
		{"one byte past the end", 0xfff, 2, true, 0},
		{"ends exactly at end of memory", 0xff1, 15, false, 1},
		{"one byte past the end, max height", 0xff2, 15, true, 0},
		{"far past the end", 0xfff, 15, true, 0},
		{"zero height at last address", 0xfff, 0, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// LD VF, 1; LD I, i; DRW V0, V0, n
			m := newMachineWithROM(t, []byte{
				0x6f, 0x01,
				0xa0 | byte(tt.i>>8), byte(tt.i),
				0xd0, tt.n,
			})
			m.mem[MemSize-1] = 0x80 // the last byte of memory is a one-pixel sprite
			step(t, m)
			step(t, m)
			pcBefore := m.pc
			ok, err := stepNoPanic(t, m)
			if !ok {
				return
			}
			// Like every other instruction, DXYN is consumed whether or not it
			// succeeds: the program counter has moved on.
			if m.pc != pcBefore+InstructionSize {
				t.Errorf("pc = %04x, want %04x", m.pc, pcBefore+InstructionSize)
			}
			if tt.wantErr {
				wantErrContaining(t, err, "memory")
				if n := litPixels(m.disp); n != 0 {
					t.Errorf("%d pixels drawn despite the error", n)
				}
				if m.regV[0xf] != 1 {
					t.Errorf("VF = %d after the error, want it left at 1", m.regV[0xf])
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if n := litPixels(m.disp); n != tt.wantLit {
				t.Errorf("%d pixels lit, want %d", n, tt.wantLit)
			}
			if m.regV[0xf] != 0 {
				t.Errorf("VF = %d, want 0 (nothing to collide with)", m.regV[0xf])
			}
		})
	}
}

// A sprite that starts at the last byte of memory must read that byte, not
// something else.
func TestDrawSpriteReadsLastByteOfMemory(t *testing.T) {
	// LD I, fff; DRW V0, V0, 1
	m := newMachineWithROM(t, []byte{0xaf, 0xff, 0xd0, 0x01})
	m.mem[MemSize-1] = 0b10100000
	step(t, m)
	step(t, m)
	if got, want := rowString(m.disp, 0)[:4], "#.#."; got != want {
		t.Errorf("row 0 = %q, want %q", got, want)
	}
}

// The sprite is read from wherever I points, including the program itself, not
// just the font.
func TestDrawSpriteReadsFromI(t *testing.T) {
	// LD I, 0x200; DRW V0, V0, 2 -- draws the program's own first two bytes
	m := newMachineWithROM(t, []byte{0xa2, 0x00, 0xd0, 0x02})
	step(t, m)
	step(t, m)
	if got, want := rowString(m.disp, 0)[:8], "#.#...#."; got != want { // 0xa2
		t.Errorf("row 0 = %q, want %q", got, want)
	}
	if got, want := rowString(m.disp, 1)[:8], "........"; got != want { // 0x00
		t.Errorf("row 1 = %q, want %q", got, want)
	}
}

// Drawing through the machine near the edge must clip and not fail.
func TestDrawSpriteNearEdgeThroughMachine(t *testing.T) {
	// LD V0, 60; LD V1, 30; LD I, font "0"; DRW V0, V1, 5
	m := newMachineWithROM(t, []byte{0x60, 60, 0x61, 30, 0xa0, 0x50, 0xd0, 0x15})
	for range 3 {
		step(t, m)
	}
	if ok, err := stepNoPanic(t, m); ok && err != nil {
		t.Fatalf("DRW near the edge returned an error: %s", err)
	}
	// Glyph "0" rows 0 and 1 (f0, 90) land on screen rows 30 and 31
	if got, want := rowString(m.disp, 30)[56:], "....####"; got != want {
		t.Errorf("row 30 = %q, want %q", got, want)
	}
	if got, want := rowString(m.disp, 31)[56:], "....#..#"; got != want {
		t.Errorf("row 31 = %q, want %q", got, want)
	}
	if n := litPixels(m.disp); n != 6 {
		t.Errorf("%d pixels lit, want 6", n)
	}
}

// DXYN wraps the start position from VX and VY around the display.
func TestDrawSpriteStartPositionWrapsThroughMachine(t *testing.T) {
	// LD V0, 66; LD V1, 33; LD I, font "0"; DRW V0, V1, 5
	m := newMachineWithROM(t, []byte{0x60, 66, 0x61, 33, 0xa0, 0x50, 0xd0, 0x15})
	for range 4 {
		step(t, m)
	}
	// (66,33) wraps to (2,1); glyph "0" is "####" over "#..#" x3 over "####"
	for y, want := range []string{"####", "#..#", "#..#", "#..#", "####"} {
		if got := rowString(m.disp, uint8(1+y))[2:6]; got != want {
			t.Errorf("row %d = %q, want %q", 1+y, got, want)
		}
	}
	if n := litPixels(m.disp); n != 14 {
		t.Errorf("%d pixels lit, want 14", n)
	}
}

// Pixels that are clipped off the edge take no part in collision detection: they
// must neither collide with pixels on the opposite edge (as they would if they
// wrapped) nor hide a collision on the visible part.
func TestDrawSpriteClippingAndVF(t *testing.T) {
	m := newMachineWithROM(t, []byte{
		0x60, 0, 0x61, 30, // LD V0, 0; LD V1, 30
		0xa0, 0x50, // LD I, font "0"
		0xd0, 0x15, // DRW V0, V1, 5: lights columns 0-3 of rows 30 and 31 (the rest is off the bottom)
		0x60, 62, // LD V0, 62
		0xd0, 0x15, // DRW V0, V1, 5: columns 62-63 visible, 64-67 clipped (they would wrap onto columns 0-3)
		0xd0, 0x15, // DRW again: erases it, colliding on the visible pixels only
	})
	for range 4 {
		step(t, m)
	}
	step(t, m) // LD V0, 62
	step(t, m) // the clipped draw
	if m.regV[0xf] != 0 {
		t.Errorf("VF = 1 after drawing a sprite whose clipped part overlaps the opposite edge")
	}
	// 6 from the first draw, and 3 visible pixels (2 in row 30, 1 in row 31)
	if n := litPixels(m.disp); n != 9 {
		t.Errorf("%d pixels lit, want 9\n%s", n, renderDisplay(m.disp))
	}
	step(t, m)
	if m.regV[0xf] != 1 {
		t.Errorf("VF = 0 when redrawing over the visible pixels, want 1")
	}
	if n := litPixels(m.disp); n != 6 {
		t.Errorf("%d pixels lit, want 6", n)
	}
}

// run8 runs `8XYn` after loading VX = a and VY = b. VF is first set to 0xaa, so
// that a flag the instruction fails to write is visible. If X or Y is VF itself,
// that register is loaded with a or b instead. X and Y may be the same register,
// but then a and b must be equal, since there is only one value to load.
func run8(t *testing.T, n, x, y, a, b byte) *Machine {
	t.Helper()
	if x == y && a != b {
		t.Fatalf("run8: X and Y are both V%x, but a = %02x and b = %02x differ", x, a, b)
	}
	var rom []byte
	steps := 0
	emit := func(hi, lo byte) {
		rom = append(rom, hi, lo)
		steps++
	}
	if x != 0xf && y != 0xf {
		emit(0x6f, 0xaa) // LD VF, aa
	}
	emit(0x60|x, a)      // LD VX, a
	emit(0x60|y, b)      // LD VY, b
	emit(0x80|x, y<<4|n) // 8XYn
	m := newMachineWithROM(t, rom)
	for range steps {
		step(t, m)
	}
	return m
}

func TestLoadAndLogicVxVy(t *testing.T) {
	tests := []struct {
		name string
		n    byte
		a, b byte
		want byte // VX afterwards
	}{
		{"8XY0 copy", 0x0, 0x12, 0x34, 0x34},
		{"8XY0 copy zero", 0x0, 0xff, 0x00, 0x00},
		{"8XY1 or", 0x1, 0xf0, 0x0f, 0xff},
		{"8XY1 or overlapping", 0x1, 0x81, 0x03, 0x83},
		{"8XY2 and", 0x2, 0xf0, 0x3c, 0x30},
		{"8XY2 and disjoint", 0x2, 0xf0, 0x0f, 0x00},
		{"8XY3 xor", 0x3, 0xff, 0x0f, 0xf0},
		{"8XY3 xor overlapping", 0x3, 0x81, 0x03, 0x82},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := run8(t, tt.n, 3, 4, tt.a, tt.b)
			if m.regV[3] != tt.want {
				t.Errorf("V3 = %02x, want %02x", m.regV[3], tt.want)
			}
			if m.regV[4] != tt.b {
				t.Errorf("V4 = %02x, want it unchanged at %02x", m.regV[4], tt.b)
			}
			for i, v := range m.regV {
				// VF is not asserted for the logic ops: whether they reset it is a
				// quirk (the original COSMAC VIP does)
				if i != 3 && i != 4 && i != 0xf && v != 0 {
					t.Errorf("V%x = %02x, want 0", i, v)
				}
			}
		})
	}
}

// With the same register on both sides, X ends up holding Y's value, so these
// reduce to operations on one value.
func TestLogicVxVxSameRegister(t *testing.T) {
	tests := []struct {
		name string
		n    byte
		want byte
	}{
		{"copy", 0x0, 0x5a},
		{"or", 0x1, 0x5a},
		{"and", 0x2, 0x5a},
		{"xor", 0x3, 0x00},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := run8(t, tt.n, 2, 2, 0x5a, 0x5a)
			if m.regV[2] != tt.want {
				t.Errorf("V2 = %02x, want %02x", m.regV[2], tt.want)
			}
		})
	}
}

func TestAddVxVy(t *testing.T) {
	tests := []struct {
		a, b  byte
		want  byte
		carry byte
	}{
		{1, 2, 3, 0},
		{0, 0, 0, 0},
		{0xff, 0, 0xff, 0},
		{0xfe, 1, 0xff, 0}, // the largest sum that does not carry
		{0xff, 1, 0x00, 1}, // the smallest sum that does
		{0x80, 0x80, 0x00, 1},
		{0xff, 0xff, 0xfe, 1},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%02x+%02x", tt.a, tt.b), func(t *testing.T) {
			m := run8(t, 0x4, 3, 4, tt.a, tt.b)
			if m.regV[3] != tt.want {
				t.Errorf("V3 = %02x, want %02x", m.regV[3], tt.want)
			}
			if m.regV[0xf] != tt.carry {
				t.Errorf("VF = %02x, want %02x", m.regV[0xf], tt.carry)
			}
			if m.regV[4] != tt.b {
				t.Errorf("V4 = %02x, want it unchanged at %02x", m.regV[4], tt.b)
			}
		})
	}
}

// 8XY5 sets VX = VX - VY, and VF to 1 when there is NO borrow (VX >= VY), 0 when
// there is one.
func TestSubVxVy(t *testing.T) {
	tests := []struct {
		a, b     byte
		want     byte
		noBorrow byte
	}{
		{5, 3, 2, 1},
		{3, 5, 0xfe, 0},
		{5, 5, 0, 1}, // equal does not borrow
		{0, 0, 0, 1},
		{0xff, 0, 0xff, 1},
		{0, 1, 0xff, 0},
		{0x80, 0x7f, 0x01, 1},
		{0x7f, 0x80, 0xff, 0},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%02x-%02x", tt.a, tt.b), func(t *testing.T) {
			m := run8(t, 0x5, 3, 4, tt.a, tt.b)
			if m.regV[3] != tt.want {
				t.Errorf("V3 = %02x, want %02x", m.regV[3], tt.want)
			}
			if m.regV[0xf] != tt.noBorrow {
				t.Errorf("VF = %02x, want %02x", m.regV[0xf], tt.noBorrow)
			}
			if m.regV[4] != tt.b {
				t.Errorf("V4 = %02x, want it unchanged at %02x", m.regV[4], tt.b)
			}
		})
	}
}

// The flag is written after the result, so when X is VF the flag wins; and when Y
// is VF the old value of VF is the operand.
func TestAddSubWithVF(t *testing.T) {
	t.Run("add, X is VF", func(t *testing.T) {
		m := run8(t, 0x4, 0xf, 1, 0xff, 0x01) // VF = ff + 01 = 00, carry
		if m.regV[0xf] != 1 {
			t.Errorf("VF = %02x, want 01 (the flag, not the sum)", m.regV[0xf])
		}
	})
	t.Run("add, X is VF, no carry", func(t *testing.T) {
		m := run8(t, 0x4, 0xf, 1, 0x01, 0x01) // VF = 01 + 01 = 02, no carry
		if m.regV[0xf] != 0 {
			t.Errorf("VF = %02x, want 00 (the flag, not the sum)", m.regV[0xf])
		}
	})
	t.Run("add, Y is VF", func(t *testing.T) {
		m := run8(t, 0x4, 0, 0xf, 0x01, 0xff) // V0 = 01 + ff = 00, carry
		if m.regV[0] != 0 || m.regV[0xf] != 1 {
			t.Errorf("V0 = %02x, VF = %02x; want 00 and 01", m.regV[0], m.regV[0xf])
		}
	})
	t.Run("sub, X is VF", func(t *testing.T) {
		m := run8(t, 0x5, 0xf, 1, 0x05, 0x03) // VF = 05 - 03 = 02, no borrow
		if m.regV[0xf] != 1 {
			t.Errorf("VF = %02x, want 01 (the flag, not the difference)", m.regV[0xf])
		}
	})
	t.Run("sub, Y is VF", func(t *testing.T) {
		m := run8(t, 0x5, 0, 0xf, 0x03, 0x05) // V0 = 03 - 05 = fe, borrow
		if m.regV[0] != 0xfe || m.regV[0xf] != 0 {
			t.Errorf("V0 = %02x, VF = %02x; want fe and 00", m.regV[0], m.regV[0xf])
		}
	})
}

// With X and Y both VF, the operand is VF's value, and the flag still wins over
// the result.
func TestAddSubBothVF(t *testing.T) {
	tests := []struct {
		name string
		n    byte
		vf   byte
		want byte
	}{
		{"add, carry", 0x4, 0x80, 1},    // 80 + 80 = 100, carry
		{"add, no carry", 0x4, 0x01, 0}, // 01 + 01 = 02, no carry
		{"sub", 0x5, 0x37, 1},           // x - x = 0, no borrow
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := run8(t, tt.n, 0xf, 0xf, tt.vf, tt.vf)
			if m.regV[0xf] != tt.want {
				t.Errorf("VF = %02x, want %02x", m.regV[0xf], tt.want)
			}
		})
	}
}

func TestAddSubSameRegister(t *testing.T) {
	// VX op VX: the operand is the value just loaded
	m := run8(t, 0x4, 2, 2, 0x80, 0x80)
	if m.regV[2] != 0 || m.regV[0xf] != 1 {
		t.Errorf("0x80 + 0x80: V2 = %02x, VF = %02x; want 00 and 01", m.regV[2], m.regV[0xf])
	}
	m = run8(t, 0x5, 2, 2, 0x80, 0x80)
	if m.regV[2] != 0 || m.regV[0xf] != 1 {
		t.Errorf("0x80 - 0x80: V2 = %02x, VF = %02x; want 00 and 01", m.regV[2], m.regV[0xf])
	}
}

// runSkip loads the registers given by setup (pairs of register number and
// value), runs the single instruction op, then runs the instruction that
// follows it, and reports whether that following instruction was skipped. The
// following instruction is `LD VE, 1` and the one after it is `LD VD, 1`, so
// exactly one of them runs.
func runSkip(t *testing.T, op uint16, setup ...[2]byte) (skipped bool, m *Machine) {
	t.Helper()
	var rom []byte
	for _, s := range setup {
		rom = append(rom, 0x60|s[0], s[1]) // LD V<reg>, value
	}
	rom = append(rom, byte(op>>8), byte(op), 0x6e, 0x01, 0x6d, 0x01)
	m = newMachineWithROM(t, rom)
	for range len(setup) {
		step(t, m)
	}
	pcAtSkip := m.pc
	step(t, m) // the skip instruction itself
	switch want := pcAtSkip + 2; {
	case m.pc == want:
		skipped = false
	case m.pc == want+2:
		skipped = true
	default:
		t.Fatalf("pc = %04x after the skip instruction at %04x, want %04x or %04x", m.pc, pcAtSkip, want, want+2)
	}
	step(t, m) // whichever instruction comes next
	switch {
	case skipped && (m.regV[0xd] != 1 || m.regV[0xe] != 0):
		t.Fatalf("skipped, but VE = %d and VD = %d; want the first instruction not to run", m.regV[0xe], m.regV[0xd])
	case !skipped && (m.regV[0xe] != 1 || m.regV[0xd] != 0):
		t.Fatalf("not skipped, but VE = %d and VD = %d; want the first instruction to run", m.regV[0xe], m.regV[0xd])
	}
	return skipped, m
}

// The skip tests below use one table per opcode family: each row loads V2 (and
// V3 for the register forms) and says whether the next instruction is skipped.

func TestSkipEqualImmediate(t *testing.T) { // 3XNN
	tests := []struct {
		name        string
		vx, nn      byte
		wantSkipped bool
	}{
		{"equal", 0x42, 0x42, true},
		{"not equal", 0x42, 0x43, false},
		{"equal zero", 0x00, 0x00, true},
		{"equal ff", 0xff, 0xff, true},
		{"differ in the low bit", 0x43, 0x42, false},
		{"differ in the high bit", 0xc2, 0x42, false},
		{"zero against nonzero", 0x00, 0x01, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skipped, m := runSkip(t, 0x3200|uint16(tt.nn), [2]byte{2, tt.vx})
			if skipped != tt.wantSkipped {
				t.Errorf("skipped = %v, want %v", skipped, tt.wantSkipped)
			}
			if m.regV[2] != tt.vx {
				t.Errorf("V2 = %02x, want it unchanged at %02x", m.regV[2], tt.vx)
			}
		})
	}
}

func TestSkipNotEqualImmediate(t *testing.T) { // 4XNN
	tests := []struct {
		name        string
		vx, nn      byte
		wantSkipped bool
	}{
		{"equal", 0x42, 0x42, false},
		{"not equal", 0x42, 0x43, true},
		{"equal zero", 0x00, 0x00, false},
		{"equal ff", 0xff, 0xff, false},
		{"differ in the low bit", 0x43, 0x42, true},
		{"differ in the high bit", 0xc2, 0x42, true},
		{"zero against nonzero", 0x00, 0x01, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skipped, m := runSkip(t, 0x4200|uint16(tt.nn), [2]byte{2, tt.vx})
			if skipped != tt.wantSkipped {
				t.Errorf("skipped = %v, want %v", skipped, tt.wantSkipped)
			}
			if m.regV[2] != tt.vx {
				t.Errorf("V2 = %02x, want it unchanged at %02x", m.regV[2], tt.vx)
			}
		})
	}
}

func TestSkipEqualRegisters(t *testing.T) { // 5XY0
	tests := []struct {
		name        string
		vx, vy      byte
		wantSkipped bool
	}{
		{"equal", 0x42, 0x42, true},
		{"not equal", 0x42, 0x43, false},
		{"equal zero", 0x00, 0x00, true},
		{"equal ff", 0xff, 0xff, true},
		{"differ in the high bit", 0xc2, 0x42, false},
		{"zero against nonzero", 0x00, 0x01, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skipped, m := runSkip(t, 0x5230, [2]byte{2, tt.vx}, [2]byte{3, tt.vy})
			if skipped != tt.wantSkipped {
				t.Errorf("skipped = %v, want %v", skipped, tt.wantSkipped)
			}
			if m.regV[2] != tt.vx || m.regV[3] != tt.vy {
				t.Errorf("V2 = %02x, V3 = %02x, want them unchanged at %02x and %02x", m.regV[2], m.regV[3], tt.vx, tt.vy)
			}
		})
	}
}

func TestSkipNotEqualRegisters(t *testing.T) { // 9XY0
	tests := []struct {
		name        string
		vx, vy      byte
		wantSkipped bool
	}{
		{"equal", 0x42, 0x42, false},
		{"not equal", 0x42, 0x43, true},
		{"equal zero", 0x00, 0x00, false},
		{"equal ff", 0xff, 0xff, false},
		{"differ in the high bit", 0xc2, 0x42, true},
		{"zero against nonzero", 0x00, 0x01, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skipped, m := runSkip(t, 0x9230, [2]byte{2, tt.vx}, [2]byte{3, tt.vy})
			if skipped != tt.wantSkipped {
				t.Errorf("skipped = %v, want %v", skipped, tt.wantSkipped)
			}
			if m.regV[2] != tt.vx || m.regV[3] != tt.vy {
				t.Errorf("V2 = %02x, V3 = %02x, want them unchanged at %02x and %02x", m.regV[2], m.regV[3], tt.vx, tt.vy)
			}
		})
	}
}

// Comparing a register with itself is always equal.
func TestSkipSameRegister(t *testing.T) {
	for _, tt := range []struct {
		op          uint16
		wantSkipped bool
	}{
		{0x5220, true},
		{0x9220, false},
	} {
		t.Run(fmt.Sprintf("%04x", tt.op), func(t *testing.T) {
			skipped, _ := runSkip(t, tt.op, [2]byte{2, 0x77})
			if skipped != tt.wantSkipped {
				t.Errorf("skipped = %v, want %v", skipped, tt.wantSkipped)
			}
		})
	}
}

// The flag register takes part in comparisons like any other, and a skip never
// writes to it.
func TestSkipWithVF(t *testing.T) {
	tests := []struct {
		name        string
		op          uint16
		vf, v2      byte // loaded before the skip; v2 is only used by the register forms
		wantSkipped bool
	}{
		// VF holds values no skip handler would plausibly write, so that a stray
		// write to it cannot look like "unchanged"
		{"3XNN with VF", 0x3fa7, 0xa7, 0, true},
		{"3XNN with VF, not equal", 0x3fa7, 0xa8, 0, false},
		{"4XNN with VF", 0x4fa7, 0xa8, 0, true},
		{"4XNN with VF, equal", 0x4fa7, 0xa7, 0, false},
		{"5XY0 with VF as X", 0x5f20, 0xb9, 0xb9, true},
		{"5XY0 with VF as Y", 0x52f0, 0xb9, 0xba, false},
		{"9XY0 with VF as X", 0x9f20, 0xb9, 0xba, true},
		{"9XY0 with VF as Y", 0x92f0, 0xb9, 0xb9, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skipped, m := runSkip(t, tt.op, [2]byte{0xf, tt.vf}, [2]byte{2, tt.v2})
			if skipped != tt.wantSkipped {
				t.Errorf("skipped = %v, want %v", skipped, tt.wantSkipped)
			}
			if m.regV[0xf] != tt.vf {
				t.Errorf("VF = %02x, want it unchanged at %02x", m.regV[0xf], tt.vf)
			}
		})
	}
}

// A skip can test the flag an arithmetic instruction just wrote.
func TestSkipOnArithmeticFlag(t *testing.T) {
	tests := []struct {
		name        string
		n           byte // 8XY4 or 8XY5
		a, b        byte
		skipIfVF    byte // 3F<this> skips when VF equals it
		wantSkipped bool
	}{
		{"carry", 0x4, 0xff, 0x01, 1, true},
		{"VF is cleared when there is no carry", 0x4, 0x01, 0x01, 0xaa, false},
		{"no carry", 0x4, 0x01, 0x01, 1, false},
		{"no carry, skip on zero", 0x4, 0x01, 0x01, 0, true},
		{"no borrow", 0x5, 0x05, 0x03, 1, true},
		{"borrow", 0x5, 0x03, 0x05, 0, true},
		{"borrow, skip on one", 0x5, 0x03, 0x05, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// LD VF, aa (a sentinel, so that a flag the instruction fails to write
			// is visible); LD V0, a; LD V1, b; 8014/8015; SE VF, x; LD VE, 1;
			// LD VD, 1
			m := newMachineWithROM(t, []byte{
				0x6f, 0xaa, 0x60, tt.a, 0x61, tt.b, 0x80, 0x10 | tt.n,
				0x3f, tt.skipIfVF, 0x6e, 0x01, 0x6d, 0x01,
			})
			for range 6 { // four set-up instructions, the skip, then whichever follows
				step(t, m)
			}
			ranFirst, ranSecond := m.regV[0xe] == 1, m.regV[0xd] == 1
			if ranFirst == ranSecond {
				t.Fatalf("VE = %d, VD = %d; want exactly one of the two loads to have run", m.regV[0xe], m.regV[0xd])
			}
			if skipped := ranSecond; skipped != tt.wantSkipped {
				t.Errorf("skipped = %v, want %v (VF = %d)", skipped, tt.wantSkipped, m.regV[0xf])
			}
		})
	}
}

// 5XYn and 9XYn are only skips when n is 0. The other values are not
// instructions, and must not behave like a skip.
func TestSkipRegistersRequireZeroLowNibble(t *testing.T) {
	for _, family := range []uint16{0x5000, 0x9000} {
		for n := uint16(1); n <= 0xf; n++ {
			op := family | 0x230 | n
			t.Run(fmt.Sprintf("%04x", op), func(t *testing.T) {
				// V2 == V3 and V2 != V3 would both skip if n were ignored
				for _, vy := range []byte{0x10, 0x11} {
					m := newMachineWithROM(t, []byte{0x62, 0x10, 0x63, vy, byte(op >> 8), byte(op), 0x6e, 0x01})
					step(t, m)
					step(t, m)
					pc := m.pc
					ok, err := stepNoPanic(t, m)
					if !ok {
						return
					}
					wantErrContaining(t, err, fmt.Sprintf("%04x", op))
					if m.pc != pc+InstructionSize {
						t.Errorf("pc = %04x, want %04x: an invalid instruction must not skip", m.pc, pc+InstructionSize)
					}
				}
			})
		}
	}
}

// A skip jumps over exactly the next instruction, and that instruction has no
// effect at all, whatever it is.
func TestSkipDoesNotExecuteSkippedInstruction(t *testing.T) {
	tests := []struct {
		name    string
		skipped []byte // the two bytes of the instruction that gets skipped
		check   func(t *testing.T, m *Machine)
	}{
		{"a call", []byte{0x22, 0x10}, func(t *testing.T, m *Machine) {
			if m.sp != 0 {
				t.Errorf("sp = %d, a skipped CALL must not push", m.sp)
			}
		}},
		{"a jump", []byte{0x1f, 0xff}, func(t *testing.T, m *Machine) {
			if m.pc != RomAddress+8 { // past LD I, SE, the skipped jump and LD V1
				t.Errorf("pc = %04x, want %04x", m.pc, RomAddress+8)
			}
		}},
		{"a draw", []byte{0xd0, 0x15}, func(t *testing.T, m *Machine) {
			if n := litPixels(m.disp); n != 0 {
				t.Errorf("%d pixels lit, a skipped DRW must draw nothing", n)
			}
		}},
		{"a load", []byte{0x65, 0x99}, func(t *testing.T, m *Machine) {
			if m.regV[5] != 0 {
				t.Errorf("V5 = %02x, a skipped LD must not run", m.regV[5])
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// LD I, font "0" (so that a draw would light pixels); SE V0, 0 (always
			// true); <skipped>; LD V1, 1
			rom := append([]byte{0xa0, 0x50, 0x30, 0x00}, tt.skipped...)
			rom = append(rom, 0x61, 0x01)
			m := newMachineWithROM(t, rom)
			step(t, m) // LD I
			step(t, m) // the skip
			step(t, m) // LD V1
			if m.regV[1] != 1 {
				t.Errorf("V1 = %d, want the instruction after the skipped one to run", m.regV[1])
			}
			tt.check(t, m)
		})
	}
}

// Two skips in a row: the second is the instruction that gets skipped, so it is
// not evaluated.
func TestSkipOverASkip(t *testing.T) {
	// SE V0, 0 (true: skips the next); SNE V0, 1 (would skip, but is skipped);
	// LD V1, 1 (must run)
	m := newMachineWithROM(t, []byte{0x30, 0x00, 0x40, 0x01, 0x61, 0x01, 0x62, 0x01})
	step(t, m)
	step(t, m)
	if m.regV[1] != 1 || m.regV[2] != 0 {
		t.Errorf("V1 = %d, V2 = %d; want 1 and 0", m.regV[1], m.regV[2])
	}
}

// A skip as the last instruction in memory is fine in itself: it leaves the
// program counter past the end, and it is the next fetch that fails.
func TestSkipAtEndOfMemory(t *testing.T) {
	m := newMachineWithROM(t, nil)
	m.pc = MemSize - InstructionSize // 0xffe
	m.mem[m.pc], m.mem[m.pc+1] = 0x30, 0x00
	if ok, err := stepNoPanic(t, m); ok && err != nil {
		t.Fatalf("skip at the last address returned an error: %s", err)
	}
	// The fetch moved pc to 0x1000, and the skip moved it on by another 2
	if want := uint16(MemSize + InstructionSize); m.pc != want {
		t.Errorf("pc = %04x, want %04x", m.pc, want)
	}
	ok, err := stepNoPanic(t, m)
	if ok {
		wantErrContaining(t, err, "memory")
	}
}

// A skip at 0xFFC lands the program counter exactly at the end of memory.
func TestSkipLandsAtEndOfMemory(t *testing.T) {
	m := newMachineWithROM(t, nil)
	m.pc = MemSize - 2*InstructionSize // 0xffc
	m.mem[m.pc], m.mem[m.pc+1] = 0x30, 0x00
	if ok, err := stepNoPanic(t, m); ok && err != nil {
		t.Fatalf("skip returned an error: %s", err)
	}
	if m.pc != MemSize {
		t.Errorf("pc = %04x, want %04x", m.pc, MemSize)
	}
	ok, err := stepNoPanic(t, m)
	if ok {
		wantErrContaining(t, err, "memory")
	}
}

// 8XY7 sets VX = VY - VX, and VF to 1 when there is NO borrow (VY >= VX).
func TestReverseSubVxVy(t *testing.T) {
	tests := []struct {
		a, b     byte // VX, VY
		want     byte
		noBorrow byte
	}{
		{3, 5, 2, 1},
		{5, 3, 0xfe, 0},
		{5, 5, 0, 1}, // equal does not borrow
		{0, 0, 0, 1},
		{0, 0xff, 0xff, 1},
		{1, 0, 0xff, 0},
		{0xff, 0, 0x01, 0},
		{0x7f, 0x80, 0x01, 1},
		{0x80, 0x7f, 0xff, 0},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%02x-%02x", tt.b, tt.a), func(t *testing.T) {
			m := run8(t, 0x7, 3, 4, tt.a, tt.b)
			if m.regV[3] != tt.want {
				t.Errorf("V3 = %02x, want %02x", m.regV[3], tt.want)
			}
			if m.regV[0xf] != tt.noBorrow {
				t.Errorf("VF = %02x, want %02x", m.regV[0xf], tt.noBorrow)
			}
			if m.regV[4] != tt.b {
				t.Errorf("V4 = %02x, want it unchanged at %02x", m.regV[4], tt.b)
			}
		})
	}
}

func TestReverseSubWithVF(t *testing.T) {
	t.Run("X is VF, the flag wins", func(t *testing.T) {
		m := run8(t, 0x7, 0xf, 1, 0x03, 0x05) // VF = 05 - 03 = 02, no borrow
		if m.regV[0xf] != 1 {
			t.Errorf("VF = %02x, want 01 (the flag, not the difference)", m.regV[0xf])
		}
	})
	t.Run("X is VF, borrow", func(t *testing.T) {
		m := run8(t, 0x7, 0xf, 1, 0x05, 0x03) // VF = 03 - 05 = fe, borrow
		if m.regV[0xf] != 0 {
			t.Errorf("VF = %02x, want 00 (the flag, not the difference)", m.regV[0xf])
		}
	})
	t.Run("Y is VF", func(t *testing.T) {
		m := run8(t, 0x7, 0, 0xf, 0x03, 0x05) // V0 = VF(05) - 03 = 02, no borrow
		if m.regV[0] != 0x02 || m.regV[0xf] != 1 {
			t.Errorf("V0 = %02x, VF = %02x; want 02 and 01", m.regV[0], m.regV[0xf])
		}
	})
	t.Run("Y is VF, borrow", func(t *testing.T) {
		m := run8(t, 0x7, 0, 0xf, 0x05, 0x03) // V0 = VF(03) - 05 = fe, borrow
		if m.regV[0] != 0xfe || m.regV[0xf] != 0 {
			t.Errorf("V0 = %02x, VF = %02x; want fe and 00", m.regV[0], m.regV[0xf])
		}
	})
	t.Run("both VF", func(t *testing.T) {
		m := run8(t, 0x7, 0xf, 0xf, 0x37, 0x37) // x - x = 0, no borrow
		if m.regV[0xf] != 1 {
			t.Errorf("VF = %02x, want 01", m.regV[0xf])
		}
	})
	t.Run("same register", func(t *testing.T) {
		m := run8(t, 0x7, 2, 2, 0x80, 0x80)
		if m.regV[2] != 0 || m.regV[0xf] != 1 {
			t.Errorf("V2 = %02x, VF = %02x; want 00 and 01", m.regV[2], m.regV[0xf])
		}
	})
}

// The shifts below are the original COSMAC VIP behaviour: 8XY6 and 8XYE shift VY
// and store the result in VX (VX's old value is ignored). Later interpreters
// shift VX in place instead; when a quirks setting for that exists these tests
// become the VIP preset.

// 8XY6 sets VX = VY >> 1, and VF to the bit that was shifted out.
func TestShiftRightVxVy(t *testing.T) {
	tests := []struct {
		vy   byte
		want byte
		vf   byte
	}{
		{0b0000_0101, 0b0000_0010, 1},
		{0b0000_0100, 0b0000_0010, 0},
		{0xff, 0x7f, 1},
		{0x01, 0x00, 1},
		{0x00, 0x00, 0},
		{0x80, 0x40, 0}, // the high bit moves down and is not the flag
		{0xfe, 0x7f, 0},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%08b", tt.vy), func(t *testing.T) {
			// VX starts at ff so that using it as the source would show
			m := run8(t, 0x6, 3, 4, 0xff, tt.vy)
			if m.regV[3] != tt.want {
				t.Errorf("V3 = %02x, want %02x", m.regV[3], tt.want)
			}
			if m.regV[0xf] != tt.vf {
				t.Errorf("VF = %02x, want %02x", m.regV[0xf], tt.vf)
			}
			if m.regV[4] != tt.vy {
				t.Errorf("V4 = %02x, want VY left unchanged at %02x", m.regV[4], tt.vy)
			}
		})
	}
}

// 8XYE sets VX = VY << 1, and VF to the bit that was shifted out (0 or 1).
func TestShiftLeftVxVy(t *testing.T) {
	tests := []struct {
		vy   byte
		want byte
		vf   byte
	}{
		{0b1000_0001, 0b0000_0010, 1},
		{0b0100_0001, 0b1000_0010, 0},
		{0xff, 0xfe, 1},
		{0x80, 0x00, 1},
		{0x00, 0x00, 0},
		{0x01, 0x02, 0}, // the low bit moves up and is not the flag
		{0x7f, 0xfe, 0},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%08b", tt.vy), func(t *testing.T) {
			m := run8(t, 0xe, 3, 4, 0xff, tt.vy)
			if m.regV[3] != tt.want {
				t.Errorf("V3 = %02x, want %02x", m.regV[3], tt.want)
			}
			if m.regV[0xf] != tt.vf {
				t.Errorf("VF = %02x, want %02x (exactly 0 or 1)", m.regV[0xf], tt.vf)
			}
			if m.regV[4] != tt.vy {
				t.Errorf("V4 = %02x, want VY left unchanged at %02x", m.regV[4], tt.vy)
			}
		})
	}
}

func TestShiftWithVF(t *testing.T) {
	tests := []struct {
		name   string
		n      byte
		x, y   byte
		a, b   byte
		wantX  byte // expected value of VX, if X is not VF
		wantVF byte
	}{
		// X is VF: the flag is written after the result, so it wins
		{"right, X is VF", 0x6, 0xf, 1, 0x00, 0x05, 0, 1},         // result 02, flag 1
		{"right, X is VF, flag 0", 0x6, 0xf, 1, 0x00, 0x06, 0, 0}, // result 03, flag 0
		{"left, X is VF", 0xe, 0xf, 1, 0x00, 0x81, 0, 1},          // result 02, flag 1
		{"left, X is VF, flag 0", 0xe, 0xf, 1, 0x00, 0x41, 0, 0},  // result 82, flag 0
		// Y is VF: its old value is the source
		{"right, Y is VF", 0x6, 3, 0xf, 0xff, 0x05, 0x02, 1},
		{"left, Y is VF", 0xe, 3, 0xf, 0xff, 0x81, 0x02, 1},
		// both VF: VF = VF shifted, then the flag
		{"right, both VF", 0x6, 0xf, 0xf, 0x05, 0x05, 0, 1},
		{"left, both VF", 0xe, 0xf, 0xf, 0x81, 0x81, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := run8(t, tt.n, tt.x, tt.y, tt.a, tt.b)
			if m.regV[0xf] != tt.wantVF {
				t.Errorf("VF = %02x, want %02x", m.regV[0xf], tt.wantVF)
			}
			if tt.x != 0xf && m.regV[tt.x] != tt.wantX {
				t.Errorf("V%x = %02x, want %02x", tt.x, m.regV[tt.x], tt.wantX)
			}
			if tt.y != 0xf && tt.y != tt.x && m.regV[tt.y] != tt.b {
				t.Errorf("V%x = %02x, want VY left unchanged at %02x", tt.y, m.regV[tt.y], tt.b)
			}
		})
	}
}

// With the same register on both sides the shift works in place.
func TestShiftSameRegister(t *testing.T) {
	m := run8(t, 0x6, 2, 2, 0x0b, 0x0b)
	if m.regV[2] != 0x05 || m.regV[0xf] != 1 {
		t.Errorf(">>: V2 = %02x, VF = %02x; want 05 and 01", m.regV[2], m.regV[0xf])
	}
	m = run8(t, 0xe, 2, 2, 0xc1, 0xc1)
	if m.regV[2] != 0x82 || m.regV[0xf] != 1 {
		t.Errorf("<<: V2 = %02x, VF = %02x; want 82 and 01", m.regV[2], m.regV[0xf])
	}
}

// BNNN jumps to NNN + V0. (Some later interpreters use VX, where X is the high
// nibble of NNN; the original uses V0.)
func TestJumpV0(t *testing.T) {
	tests := []struct {
		name string
		v0   byte
		nnn  uint16
		want uint16
	}{
		{"V0 is zero", 0x00, 0x300, 0x300},
		{"small offset", 0x10, 0x300, 0x310},
		{"largest offset", 0xff, 0x200, 0x2ff},
		{"carries into the next nibble", 0x20, 0x3f0, 0x410},
		{"lowest address", 0x00, 0x000, 0x000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// LD V0, v0; JP V0, nnn
			m := newMachineWithROM(t, []byte{0x60, tt.v0, 0xb0 | byte(tt.nnn>>8), byte(tt.nnn)})
			step(t, m)
			step(t, m)
			if m.pc != tt.want {
				t.Errorf("pc = %04x, want %04x", m.pc, tt.want)
			}
			if m.sp != 0 {
				t.Errorf("sp = %d, a jump must not push a return address", m.sp)
			}
			if m.regV[0] != tt.v0 {
				t.Errorf("V0 = %02x, want it unchanged at %02x", m.regV[0], tt.v0)
			}
		})
	}
}

// The offset is always V0, not the register named by the high nibble of NNN.
func TestJumpV0IgnoresOtherRegisters(t *testing.T) {
	// LD V0, 10; LD V3, 40; JP V0, 305
	m := newMachineWithROM(t, []byte{0x60, 0x10, 0x63, 0x40, 0xb3, 0x05})
	for range 3 {
		step(t, m)
	}
	if m.pc != 0x315 {
		t.Errorf("pc = %04x, want 0315 (V0 = 10 is the offset, not V3 = 40)", m.pc)
	}
}

// NNN + V0 can exceed the end of memory. The address is not wrapped to 12 bits:
// the jump itself succeeds, and it is the next fetch that fails.
func TestJumpV0PastEndOfMemory(t *testing.T) {
	// LD V0, ff; JP V0, fff
	m := newMachineWithROM(t, []byte{0x60, 0xff, 0xbf, 0xff})
	step(t, m)
	if ok, err := stepNoPanic(t, m); !ok || err != nil {
		t.Fatalf("the jump itself returned %v (panicked: %v)", err, !ok)
	}
	if m.pc != 0x10fe {
		t.Errorf("pc = %04x, want 10fe (fff + ff, not wrapped)", m.pc)
	}
	ok, err := stepNoPanic(t, m)
	if ok {
		wantErrContaining(t, err, "memory")
	}
}

// A jump to exactly the end of memory behaves the same way.
func TestJumpV0ToEndOfMemory(t *testing.T) {
	// LD V0, 01; JP V0, fff
	m := newMachineWithROM(t, []byte{0x60, 0x01, 0xbf, 0xff})
	step(t, m)
	step(t, m)
	if m.pc != MemSize {
		t.Errorf("pc = %04x, want %04x", m.pc, MemSize)
	}
	ok, err := stepNoPanic(t, m)
	if ok {
		wantErrContaining(t, err, "memory")
	}
}

// ---------------------------------------------------------------------------
// FX1E, FX33, FX55, FX65
// ---------------------------------------------------------------------------

// runF runs a single FXnn instruction on a fresh machine, after setup has set
// the machine's state, and returns the machine and the instruction's error.
func runF(t *testing.T, x byte, nn byte, setup func(m *Machine)) (*Machine, error) {
	t.Helper()
	m := newMachineWithROM(t, []byte{0xf0 | x, nn})
	if setup != nil {
		setup(m)
	}
	ok, err := stepNoPanic(t, m)
	if !ok {
		t.FailNow()
	}
	return m, err
}

func TestAddIVx(t *testing.T) { // FX1E
	cases := []struct {
		name  string
		x     byte
		i     uint16
		vx    uint8
		wantI uint16
	}{
		{"simple", 3, 0x300, 0x10, 0x310},
		{"zero", 3, 0x300, 0, 0x300},
		{"from zero", 0, 0, 0xff, 0xff},
		{"max reachable", 3, 0xf00, 0xff, 0xfff},
		{"uses VF as the source", 0xf, 0x200, 0x07, 0x207},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := runF(t, tc.x, 0x1e, func(m *Machine) {
				m.regI = tc.i
				m.regV[tc.x] = tc.vx
			})
			if err != nil {
				t.Fatal(err)
			}
			if m.regI != tc.wantI {
				t.Errorf("I = %03x, want %03x", m.regI, tc.wantI)
			}
			if m.regV[tc.x] != tc.vx {
				t.Errorf("V%X = %02x, want it unchanged (%02x)", tc.x, m.regV[tc.x], tc.vx)
			}
		})
	}
}

// FX1E must leave VF alone, both when it succeeds and when it fails. (Some
// interpreters set VF when I overflows; this one reports an error instead.)
func TestAddIVxLeavesVF(t *testing.T) {
	cases := []struct {
		name    string
		i       uint16
		vx      uint8
		wantErr bool
	}{
		{"success", 0xf00, 0xff, false},
		{"failure", 0xf01, 0xff, true}, // would reach 0x1000
	}
	for _, tc := range cases {
		for _, vf := range []uint8{0, 1, 0x55} {
			t.Run(fmt.Sprintf("%s/VF=%02x", tc.name, vf), func(t *testing.T) {
				m, err := runF(t, 1, 0x1e, func(m *Machine) {
					m.regI = tc.i
					m.regV[1] = tc.vx
					m.regV[0xf] = vf
				})
				if (err != nil) != tc.wantErr {
					t.Fatalf("error = %v, want error: %v", err, tc.wantErr)
				}
				if m.regV[0xf] != vf {
					t.Errorf("VF = %02x, want it unchanged (%02x)", m.regV[0xf], vf)
				}
			})
		}
	}
}

// I may not be advanced past the last valid address (0xfff). That includes
// landing exactly on 0x1000, which is not an address, as well as going far past.
func TestAddIVxPastEndOfMemory(t *testing.T) {
	cases := []struct {
		name string
		i    uint16
		vx   uint8
	}{
		{"exactly one past the end", 0xf01, 0xff},
		{"exactly one past the end from the last address", 0xfff, 0x01},
		{"well past the end", 0xfff, 0x10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := runF(t, 1, 0x1e, func(m *Machine) {
				m.regI = tc.i
				m.regV[1] = tc.vx
			})
			wantErrContaining(t, err, "past end of memory")
			if m.regI != tc.i {
				t.Errorf("I = %03x after a failed FX1E, want it unchanged (%03x)", m.regI, tc.i)
			}
		})
	}
}

func TestBCD(t *testing.T) { // FX33
	cases := []struct {
		v          uint8
		h, tn, one byte
	}{
		{0, 0, 0, 0},
		{5, 0, 0, 5},
		{9, 0, 0, 9},
		{10, 0, 1, 0},
		{99, 0, 9, 9},
		{100, 1, 0, 0},
		{101, 1, 0, 1},
		{123, 1, 2, 3},
		{200, 2, 0, 0},
		{254, 2, 5, 4},
		{255, 2, 5, 5},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.v), func(t *testing.T) {
			m, err := runF(t, 4, 0x33, func(m *Machine) {
				m.regI = 0x400
				m.regV[4] = tc.v
				// Sentinels around the target, to catch writes outside it
				m.mem[0x3ff] = 0xaa
				m.mem[0x403] = 0xbb
			})
			if err != nil {
				t.Fatal(err)
			}
			got := [3]byte{m.mem[0x400], m.mem[0x401], m.mem[0x402]}
			if want := [3]byte{tc.h, tc.tn, tc.one}; got != want {
				t.Errorf("BCD of %d = %v, want %v", tc.v, got, want)
			}
			if m.mem[0x3ff] != 0xaa || m.mem[0x403] != 0xbb {
				t.Errorf("wrote outside I..I+2: %02x, %02x", m.mem[0x3ff], m.mem[0x403])
			}
			if m.regI != 0x400 {
				t.Errorf("I = %03x, want it unchanged (0x400)", m.regI)
			}
			if m.regV[4] != tc.v {
				t.Errorf("V4 = %d, want it unchanged (%d)", m.regV[4], tc.v)
			}
		})
	}
}

func TestBCDWithVFAsSource(t *testing.T) {
	m, err := runF(t, 0xf, 0x33, func(m *Machine) {
		m.regI = 0x400
		m.regV[0xf] = 145
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := [3]byte{m.mem[0x400], m.mem[0x401], m.mem[0x402]}, [3]byte{1, 4, 5}; got != want {
		t.Errorf("BCD of VF = %v, want %v", got, want)
	}
}

func TestBCDAtEndOfMemory(t *testing.T) {
	// The last valid place to write three bytes
	m, err := runF(t, 0, 0x33, func(m *Machine) {
		m.regI = MemSize - 3
		m.regV[0] = 123
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := [3]byte{m.mem[MemSize-3], m.mem[MemSize-2], m.mem[MemSize-1]}, [3]byte{1, 2, 3}; got != want {
		t.Errorf("BCD at the end of memory = %v, want %v", got, want)
	}

	// One byte too far: must be an error, not a panic or a partial write
	m, err = runF(t, 0, 0x33, func(m *Machine) {
		m.regI = MemSize - 2
		m.regV[0] = 123
	})
	wantErrContaining(t, err, "past end of memory")
	if m.mem[MemSize-2] != 0 || m.mem[MemSize-1] != 0 {
		t.Errorf("partial write after a failed FX33: %02x %02x", m.mem[MemSize-2], m.mem[MemSize-1])
	}
}

func TestStoreRegisters(t *testing.T) { // FX55
	// Registers hold distinct non-zero values, so a wrong index shows up
	setRegs := func(m *Machine) {
		for i := range m.regV {
			m.regV[i] = uint8(0x10 + i)
		}
	}
	for _, x := range []byte{0, 1, 7, 0xe, 0xf} {
		t.Run(fmt.Sprintf("V0-V%X", x), func(t *testing.T) {
			m, err := runF(t, x, 0x55, func(m *Machine) {
				setRegs(m)
				m.regI = 0x400
				m.mem[0x3ff] = 0xaa
				m.mem[0x400+int(x)+1] = 0xbb // first byte past the range
			})
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i <= int(x); i++ {
				if got, want := m.mem[0x400+i], uint8(0x10+i); got != want {
					t.Errorf("mem[%03x] = %02x, want %02x (V%X)", 0x400+i, got, want, i)
				}
			}
			if m.mem[0x3ff] != 0xaa {
				t.Errorf("wrote before I: %02x", m.mem[0x3ff])
			}
			if m.mem[0x400+int(x)+1] != 0xbb {
				t.Errorf("wrote past Vx (%02x): register count is off by one", m.mem[0x400+int(x)+1])
			}
			if want := uint16(0x400 + int(x) + 1); m.regI != want {
				t.Errorf("I = %03x, want %03x (I + x + 1)", m.regI, want)
			}
			for i := range m.regV {
				if m.regV[i] != uint8(0x10+i) {
					t.Errorf("V%X = %02x, want it unchanged", i, m.regV[i])
				}
			}
		})
	}
}

func TestStoreRegistersBounds(t *testing.T) {
	setRegs := func(m *Machine) {
		for i := range m.regV {
			m.regV[i] = 0x80 + uint8(i)
		}
	}
	t.Run("last valid address", func(t *testing.T) {
		// V0-VF into the final 16 bytes of memory
		m, err := runF(t, 0xf, 0x55, func(m *Machine) {
			setRegs(m)
			m.regI = MemSize - 16
		})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := m.mem[MemSize-1], uint8(0x8f); got != want {
			t.Errorf("last byte of memory = %02x, want %02x", got, want)
		}
		if m.regI != MemSize {
			t.Errorf("I = %03x, want %03x", m.regI, MemSize)
		}
	})
	t.Run("one byte too far", func(t *testing.T) {
		m, err := runF(t, 0xf, 0x55, func(m *Machine) {
			setRegs(m)
			m.regI = MemSize - 15
		})
		wantErrContaining(t, err, "past end of memory")
		if m.regI != MemSize-15 {
			t.Errorf("I = %03x after a failed FX55, want it unchanged", m.regI)
		}
		for i := MemSize - 15; i < MemSize; i++ {
			if m.mem[i] != 0 {
				t.Fatalf("partial write after a failed FX55: mem[%03x] = %02x", i, m.mem[i])
			}
		}
	})
}

func TestLoadRegisters(t *testing.T) { // FX65
	fillMem := func(m *Machine) {
		for i := range 16 {
			m.mem[0x400+i] = uint8(0x40 + i)
		}
	}
	for _, x := range []byte{0, 1, 7, 0xe, 0xf} {
		t.Run(fmt.Sprintf("V0-V%X", x), func(t *testing.T) {
			m, err := runF(t, x, 0x65, func(m *Machine) {
				fillMem(m)
				m.regI = 0x400
				for i := range m.regV {
					m.regV[i] = 0xee
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			for i := range m.regV {
				want := uint8(0xee) // beyond Vx: untouched
				if i <= int(x) {
					want = uint8(0x40 + i)
				}
				if m.regV[i] != want {
					t.Errorf("V%X = %02x, want %02x", i, m.regV[i], want)
				}
			}
			if want := uint16(0x400 + int(x) + 1); m.regI != want {
				t.Errorf("I = %03x, want %03x (I + x + 1)", m.regI, want)
			}
			for i := range 16 {
				if m.mem[0x400+i] != uint8(0x40+i) {
					t.Errorf("mem[%03x] changed to %02x", 0x400+i, m.mem[0x400+i])
				}
			}
		})
	}
}

func TestLoadRegistersBounds(t *testing.T) {
	t.Run("last valid address", func(t *testing.T) {
		m, err := runF(t, 0xf, 0x65, func(m *Machine) {
			m.regI = MemSize - 16
			m.mem[MemSize-1] = 0x99
		})
		if err != nil {
			t.Fatal(err)
		}
		if m.regV[0xf] != 0x99 {
			t.Errorf("VF = %02x, want 99 (the last byte of memory)", m.regV[0xf])
		}
		if m.regI != MemSize {
			t.Errorf("I = %03x, want %03x", m.regI, MemSize)
		}
	})
	t.Run("one byte too far", func(t *testing.T) {
		m, err := runF(t, 0xf, 0x65, func(m *Machine) {
			m.regI = MemSize - 15
			m.regV[0] = 0x77
		})
		wantErrContaining(t, err, "past end of memory")
		if m.regI != MemSize-15 {
			t.Errorf("I = %03x after a failed FX65, want it unchanged", m.regI)
		}
		if m.regV[0] != 0x77 {
			t.Errorf("V0 = %02x after a failed FX65, want it unchanged", m.regV[0])
		}
	})
}

// A store followed by a load through the same I round-trips every register.
func TestStoreLoadRoundTrip(t *testing.T) {
	m := newMachineWithROM(t, []byte{
		0xa4, 0x00, // LD I, 0x400
		0xff, 0x55, // LD [I], VF
		0xa4, 0x00, // LD I, 0x400
		0xff, 0x65, // LD VF, [I]
	})
	for i := range m.regV {
		m.regV[i] = uint8(i*17 + 3)
	}
	want := m.regV
	step(t, m)
	step(t, m)
	m.regV = [16]uint8{}
	step(t, m)
	step(t, m)
	if m.regV != want {
		t.Errorf("registers after round trip = %v, want %v", m.regV, want)
	}
}

// ---------------------------------------------------------------------------
// CXKK
// ---------------------------------------------------------------------------

// seededMachine returns a machine whose random numbers come from a fixed seed, so
// that a test sees the same sequence on every run.
func seededMachine(seed uint64) *Machine {
	return NewMachine(WithRng(rand.New(rand.NewPCG(seed, seed))))
}

// draw runs a single CXKK instruction on the machine and returns the value it
// put in VX. It rewinds the program counter first, so it can be called
// repeatedly on the same machine.
func draw(t *testing.T, m *Machine, x, mask byte) uint8 {
	t.Helper()
	m.mem[RomAddress] = 0xc0 | x
	m.mem[RomAddress+1] = mask
	m.pc = RomAddress
	step(t, m)
	return m.regV[x]
}

// The result must never have a bit set that the mask clears, and a zero mask must
// always give zero.
func TestRandMask(t *testing.T) {
	for _, mask := range []byte{0x00, 0x01, 0x0f, 0xf0, 0x55, 0xaa, 0x81, 0xfe, 0xff} {
		t.Run(fmt.Sprintf("%02x", mask), func(t *testing.T) {
			m := seededMachine(1)
			for range 2000 {
				if got := draw(t, m, 3, mask); got&^mask != 0 {
					t.Fatalf("CXKK with mask %02x gave %02x, which has bits outside the mask", mask, got)
				}
			}
		})
	}
}

// Over many draws, every value the mask allows must turn up, and nothing else.
// With a full mask that means all 256 values, including 0x00 and 0xff, so a
// generator that stops one short of the top (or bottom) of the range fails. The
// seed is fixed, so this is deterministic; 20000 draws leave a negligible chance
// that a correct generator misses a value.
func TestRandCoversEveryAllowedValue(t *testing.T) {
	for _, mask := range []byte{0xff, 0x0f, 0xf0, 0x81, 0x00} {
		t.Run(fmt.Sprintf("%02x", mask), func(t *testing.T) {
			m := seededMachine(2)
			seen := map[uint8]bool{}
			for range 20000 {
				seen[draw(t, m, 0, mask)] = true
			}
			if want := 1 << bits.OnesCount8(mask); len(seen) != want {
				t.Errorf("saw %d distinct values with mask %02x, want %d", len(seen), mask, want)
			}
			if mask == 0xff {
				for _, v := range []uint8{0x00, 0x7f, 0x80, 0xff} {
					if !seen[v] {
						t.Errorf("never saw %02x", v)
					}
				}
			}
		})
	}
}

// Each bit should come up both set and clear, independently of the others.
func TestRandEveryBitVaries(t *testing.T) {
	m := seededMachine(3)
	var ones, zeros uint8
	for range 500 {
		got := draw(t, m, 1, 0xff)
		ones |= got
		zeros |= ^got
	}
	if ones != 0xff {
		t.Errorf("bits that were never set: %08b", ^ones)
	}
	if zeros != 0xff {
		t.Errorf("bits that were never clear: %08b", ^zeros)
	}
}

// CXKK writes VX and nothing else, including when X is VF.
func TestRandOnlyWritesVx(t *testing.T) {
	for x := range byte(16) {
		t.Run(fmt.Sprintf("V%X", x), func(t *testing.T) {
			// A zero mask must make VX exactly 0 whatever it held before.
			m := seededMachine(4)
			for i := range m.regV {
				m.regV[i] = 0xee
			}
			m.regI = 0x123
			before := m.regV
			draw(t, m, x, 0x00)
			if m.regV[x] != 0 {
				t.Errorf("V%X = %02x, want 0 with a zero mask", x, m.regV[x])
			}
			for i := range m.regV {
				if byte(i) != x && m.regV[i] != before[i] {
					t.Errorf("V%X changed from %02x to %02x", i, before[i], m.regV[i])
				}
			}
			if m.regI != 0x123 {
				t.Errorf("I = %03x, want it unchanged", m.regI)
			}
			if m.pc != RomAddress+InstructionSize {
				t.Errorf("pc = %03x, want %03x", m.pc, RomAddress+InstructionSize)
			}

			// A full mask must put the drawn values in VX, and only there. Over
			// several draws VX has to take more than one value, which it cannot do
			// if the result is routed to a different register.
			m = seededMachine(4)
			for i := range m.regV {
				m.regV[i] = 0
			}
			seen := map[uint8]bool{}
			for range 32 {
				seen[draw(t, m, x, 0xff)] = true
				for i := range m.regV {
					if byte(i) != x && m.regV[i] != 0 {
						t.Fatalf("V%X = %02x after a draw into V%X, want 0", i, m.regV[i], x)
					}
				}
			}
			if len(seen) < 2 {
				t.Errorf("V%X took %d distinct value(s) over 32 draws with a full mask", x, len(seen))
			}
		})
	}
}

// Consecutive CXKKs must draw new numbers rather than repeating one.
func TestRandAdvancesSequence(t *testing.T) {
	m := seededMachine(5)
	first := draw(t, m, 0, 0xff)
	repeats := 0
	for range 50 {
		if draw(t, m, 0, 0xff) == first {
			repeats++
		}
	}
	if repeats == 50 {
		t.Errorf("every draw gave %02x", first)
	}
}

// The injected source is what CXKK uses: the same seed gives the same sequence,
// and a different seed gives a different one.
func TestRandUsesInjectedSource(t *testing.T) {
	sequence := func(seed uint64) [32]uint8 {
		m := seededMachine(seed)
		var out [32]uint8
		for i := range out {
			out[i] = draw(t, m, 2, 0xff)
		}
		return out
	}
	if a, b := sequence(7), sequence(7); a != b {
		t.Errorf("same seed gave different sequences:\n%v\n%v", a, b)
	}
	if a, b := sequence(7), sequence(8); a == b {
		t.Errorf("different seeds gave the same sequence: %v", a)
	}
}

// The default source must be seeded differently for each machine. Two machines
// built without WithRng drawing the same 8 bytes by chance has odds of 2^-64,
// so this does not flake; a fixed default seed fails it every time.
func TestRandDefaultSourceIsSeededPerMachine(t *testing.T) {
	first8 := func(m *Machine) [8]uint8 {
		var out [8]uint8
		for i := range out {
			out[i] = draw(t, m, 0, 0xff)
		}
		return out
	}
	for name, mk := range map[string]func() *Machine{
		"no option":  func() *Machine { return NewMachine() },
		"nil source": func() *Machine { return NewMachine(WithRng(nil)) },
	} {
		t.Run(name, func(t *testing.T) {
			if a, b := first8(mk()), first8(mk()); a == b {
				t.Errorf("two default machines drew the same sequence: %v", a)
			}
		})
	}
}

// Load resets the machine but keeps its configuration, so the random source
// survives it, and carries on from where it was rather than starting over.
func TestRandSourceSurvivesLoad(t *testing.T) {
	// Two consecutive draws from an identically seeded machine that is never
	// reloaded
	ref := seededMachine(9)
	want1 := draw(t, ref, 0, 0xff)
	want2 := draw(t, ref, 0, 0xff)
	if want1 == want2 {
		t.Fatalf("seed 9 gives the same first two draws (%02x); pick a seed that doesn't", want1)
	}

	m := seededMachine(9)
	if got := draw(t, m, 0, 0xff); got != want1 {
		t.Fatalf("first draw = %02x, want %02x", got, want1)
	}
	if err := m.Load(bytes.NewReader([]byte{0xc0, 0xff})); err != nil {
		t.Fatal(err)
	}
	step(t, m)
	if m.regV[0] != want2 {
		t.Errorf("draw after Load = %02x, want %02x (the second draw, not a restart, which would give %02x)",
			m.regV[0], want2, want1)
	}
}
