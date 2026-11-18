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
