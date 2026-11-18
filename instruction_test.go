package gremul8

import "testing"

func TestInstructionDecoding(t *testing.T) {
	tests := []struct {
		name    string
		instr   instruction
		nibbles [4]uint8
		address uint16
		lower   uint8
		all     uint16
	}{
		{"all distinct", instruction{0x12, 0x34}, [4]uint8{1, 2, 3, 4}, 0x234, 0x34, 0x1234},
		{"all ones", instruction{0xff, 0xff}, [4]uint8{15, 15, 15, 15}, 0xfff, 0xff, 0xffff},
		{"zero", instruction{0x00, 0x00}, [4]uint8{}, 0, 0, 0},
		{"clear", instruction{0x00, 0xe0}, [4]uint8{0, 0, 0xe, 0}, 0x0e0, 0xe0, 0x00e0},
		{"draw", instruction{0xda, 0xb5}, [4]uint8{0xd, 0xa, 0xb, 5}, 0xab5, 0xb5, 0xdab5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, want := range tt.nibbles {
				if got := tt.instr.nibble(i); got != want {
					t.Errorf("nibble(%d) = %x, want %x", i, got, want)
				}
			}
			if got := tt.instr.nibbles(); got != tt.nibbles {
				t.Errorf("nibbles() = %x, want %x", got, tt.nibbles)
			}
			if got := tt.instr.address(); got != tt.address {
				t.Errorf("address() = %03x, want %03x", got, tt.address)
			}
			if got := tt.instr.lower(); got != tt.lower {
				t.Errorf("lower() = %02x, want %02x", got, tt.lower)
			}
			if got := tt.instr.all(); got != tt.all {
				t.Errorf("all() = %04x, want %04x", got, tt.all)
			}
		})
	}
}

func TestInstructionNibbleOutOfRangePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("nibble(4) did not panic")
		}
	}()
	instruction{0x12, 0x34}.nibble(4)
}
