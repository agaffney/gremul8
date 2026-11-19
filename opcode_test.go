package gremul8

import (
	"fmt"
	"math/bits"
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
		0x8001, // not implemented yet
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
