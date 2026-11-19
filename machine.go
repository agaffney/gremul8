package gremul8

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const (
	MemSize            = 4096 // 4Kib
	BuiltinFontAddress = 0x050
	RomAddress         = 0x200
	StackMaxDepth      = 30
)

type Machine struct {
	mem  [MemSize]byte
	disp Display
	regV [16]uint8
	regI uint16
	pc   uint16
	sp   uint16
}

func NewMachine(d Display) *Machine {
	m := &Machine{
		disp: d,
	}
	if m.disp == nil {
		m.disp = NewDummyDisplay(64, 32)
	}
	m.reset()
	return m
}

func (m *Machine) Load(r io.Reader) error {
	m.reset()
	prgm, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if len(prgm)+RomAddress > MemSize {
		return errors.New("program size exceeds memory size")
	}
	copy(m.mem[RomAddress:], prgm)
	return nil
}

func (m *Machine) reset() {
	m.disp.Clear()
	m.mem = [MemSize]byte{}
	m.pc = RomAddress
	m.sp = 0
	m.regV = [16]uint8{}
	m.regI = 0
	for i, ch := range BuiltinFont {
		copy(m.mem[BuiltinFontAddress+(i*BuiltinFontWidth):], ch[:])
	}
}

func (m *Machine) Step() error {
	if m.pc+InstructionSize > MemSize {
		return errors.New("program counter past end of memory")
	}
	// Get next instruction and evaluate it
	instr := instruction(m.mem[m.pc : m.pc+InstructionSize])
	// Increment program counter
	m.pc += 2
	if err := m.evalInstr(instr); err != nil {
		return err
	}
	return nil
}

func (m *Machine) evalInstr(instr instruction) error {
	switch instr.nibble(0) {
	case 0x0:
		return m.evalInstr0(instr)
	case 0x1:
		return m.evalInstr1(instr)
	case 0x2:
		return m.evalInstr2(instr)
	case 0x3:
		return m.evalInstr3(instr)
	case 0x4:
		return m.evalInstr4(instr)
	case 0x5:
		return m.evalInstr5(instr)
	case 0x6:
		return m.evalInstr6(instr)
	case 0x7:
		return m.evalInstr7(instr)
	case 0x8:
		return m.evalInstr8(instr)
	case 0x9:
		return m.evalInstr9(instr)
	case 0xa:
		return m.evalInstrA(instr)
	case 0xd:
		return m.evalInstrD(instr)
	default:
		return fmt.Errorf("unknown instruction: %04x", instr.all())
	}
}

func (m *Machine) evalInstr0(instr instruction) error {
	all := instr.all()
	switch {
	case all == InstrClear:
		m.disp.Clear()
	case all == InstrReturn:
		if m.sp == 0 {
			return errors.New("return instruction with empty stack")
		}
		// Set program counter to return address from top of stack
		m.pc = binary.BigEndian.Uint16(m.mem[m.sp-2 : m.sp])
		// Remove return address from stack
		m.sp -= 2
	default:
		return fmt.Errorf("unknown instruction: %04x", instr.all())
	}
	return nil
}

func (m *Machine) evalInstr1(instr instruction) error {
	m.pc = instr.address()
	return nil
}

func (m *Machine) evalInstr2(instr instruction) error {
	if m.sp/InstructionSize >= StackMaxDepth {
		return errors.New("reached maximum stack depth")
	}
	// Write next instruction address to top of stack
	binary.BigEndian.PutUint16(m.mem[m.sp:], m.pc)
	// Increment stack pointer
	m.sp += InstructionSize
	// Set the program counter to the specified address
	m.pc = instr.address()
	return nil
}

func (m *Machine) evalInstr3(instr instruction) error {
	regX := instr.nibble(1)
	val := instr.lower()
	if m.regV[regX] == val {
		m.pc += InstructionSize
	}
	return nil
}

func (m *Machine) evalInstr4(instr instruction) error {
	regX := instr.nibble(1)
	val := instr.lower()
	if m.regV[regX] != val {
		m.pc += InstructionSize
	}
	return nil
}

func (m *Machine) evalInstr5(instr instruction) error {
	switch instr.nibble(3) {
	case 0x0:
		regX := instr.nibble(1)
		regY := instr.nibble(2)
		if m.regV[regX] == m.regV[regY] {
			m.pc += InstructionSize
		}
	default:
		return fmt.Errorf("unknown instruction: %04x", instr.all())
	}
	return nil
}

func (m *Machine) evalInstr6(instr instruction) error {
	vNum := instr.nibble(1)
	val := instr.lower()
	m.regV[vNum] = val
	return nil
}

func (m *Machine) evalInstr7(instr instruction) error {
	vNum := instr.nibble(1)
	val := instr.lower()
	m.regV[vNum] += val
	return nil
}

func (m *Machine) evalInstr8(instr instruction) error {
	regX := instr.nibble(1)
	regY := instr.nibble(2)
	switch instr.nibble(3) {
	case 0x0:
		m.regV[regX] = m.regV[regY]
	case 0x1:
		m.regV[regX] |= m.regV[regY]
	case 0x2:
		m.regV[regX] &= m.regV[regY]
	case 0x3:
		m.regV[regX] ^= m.regV[regY]
	case 0x4:
		overflow := (int(m.regV[regY]) + int(m.regV[regX])) > math.MaxUint8
		m.regV[regX] += m.regV[regY]
		m.regV[0xf] = 0
		if overflow {
			m.regV[0xf] = 1
		}
	case 0x5:
		underflow := m.regV[regY] > m.regV[regX]
		m.regV[regX] -= m.regV[regY]
		m.regV[0xf] = 1
		if underflow {
			m.regV[0xf] = 0
		}
	default:
		return fmt.Errorf("unknown instruction: %04x", instr.all())
	}
	return nil
}

func (m *Machine) evalInstr9(instr instruction) error {
	switch instr.nibble(3) {
	case 0x0:
		regX := instr.nibble(1)
		regY := instr.nibble(2)
		if m.regV[regX] != m.regV[regY] {
			m.pc += InstructionSize
		}
	default:
		return fmt.Errorf("unknown instruction: %04x", instr.all())
	}
	return nil
}

func (m *Machine) evalInstrA(instr instruction) error {
	m.regI = instr.address()
	return nil
}

func (m *Machine) evalInstrD(instr instruction) error {
	regX := instr.nibble(1)
	regY := instr.nibble(2)
	size := instr.nibble(3)
	if m.regI+uint16(size) > MemSize {
		return errors.New("read past end of memory")
	}
	sprite := m.mem[m.regI : m.regI+uint16(size)]
	if m.disp.DrawSprite(m.regV[regX], m.regV[regY], sprite) {
		// Set VF to 1 on collision
		m.regV[0xf] = 1
	} else {
		// Reset VF
		m.regV[0xf] = 0
	}
	return nil
}
