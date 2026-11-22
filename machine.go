package gremul8

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

const (
	MemSize            = 4096 // 4Kib
	BuiltinFontAddress = 0x050
	RomAddress         = 0x200
	StackMaxDepth      = 30
	TickDuration       = time.Second / 60
)

type Machine struct {
	mu         sync.Mutex
	done       chan struct{}
	mem        [MemSize]byte
	disp       Display
	rng        *rand.Rand
	regV       [16]uint8
	regI       uint16
	pc         uint16
	sp         uint16
	delayTimer uint8
	soundTimer uint8
	autoTick   bool
	autoTicker *time.Ticker
}

func NewMachine(opts ...Option) *Machine {
	m := &Machine{
		disp: NewDummyDisplay(64, 32),
		rng:  rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
		done: make(chan struct{}),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}
	m.reset()
	return m
}

func (m *Machine) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case <-m.done:
		return
	default:
	}
	if m.autoTicker != nil {
		m.autoTicker.Stop()
	}
	close(m.done)
}

func (m *Machine) Load(r io.Reader) error {
	prgm, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if len(prgm)+RomAddress > MemSize {
		return errors.New("program size exceeds memory size")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case <-m.done:
		return errors.New("Close() has already been called")
	default:
	}
	m.reset()
	copy(m.mem[RomAddress:], prgm)
	return nil
}

func (m *Machine) reset() {
	if m.autoTicker != nil {
		m.autoTicker.Stop()
	}
	m.disp.Clear()
	m.mem = [MemSize]byte{}
	m.pc = RomAddress
	m.sp = 0
	m.delayTimer = 0
	m.soundTimer = 0
	m.regV = [16]uint8{}
	m.regI = 0
	for i, ch := range BuiltinFont {
		copy(m.mem[BuiltinFontAddress+(i*BuiltinFontWidth):], ch[:])
	}
	if m.autoTick {
		if m.autoTicker == nil {
			m.autoTicker = time.NewTicker(TickDuration)
			go func() {
				for {
					select {
					case <-m.autoTicker.C:
					case <-m.done:
						return
					}
					m.mu.Lock()
					m.tick()
					m.mu.Unlock()
				}
			}()
		}
		m.autoTicker.Reset(TickDuration)
	}
}

func (m *Machine) tick() {
	if m.delayTimer > 0 {
		m.delayTimer--
	}
	if m.soundTimer > 0 {
		m.soundTimer--
	}
}

func (m *Machine) Tick() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tick()
}

func (m *Machine) Step() error {
	m.mu.Lock()
	defer m.mu.Unlock()
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
	case 0xb:
		return m.evalInstrB(instr)
	case 0xc:
		return m.evalInstrC(instr)
	case 0xd:
		return m.evalInstrD(instr)
	case 0xf:
		return m.evalInstrF(instr)
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
	case 0x6:
		m.regV[regX] = m.regV[regY]
		shiftedBit := m.regV[regX] & 0x01
		m.regV[regX] = m.regV[regX] >> 1
		m.regV[0xf] = shiftedBit
	case 0x7:
		underflow := m.regV[regX] > m.regV[regY]
		m.regV[regX] = m.regV[regY] - m.regV[regX]
		m.regV[0xf] = 1
		if underflow {
			m.regV[0xf] = 0
		}
	case 0xe:
		m.regV[regX] = m.regV[regY]
		shiftedBit := m.regV[regX] & 0x80
		m.regV[regX] = m.regV[regX] << 1
		m.regV[0xf] = shiftedBit >> 7
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

func (m *Machine) evalInstrB(instr instruction) error {
	m.pc = instr.address() + uint16(m.regV[0x0])
	return nil
}

func (m *Machine) evalInstrC(instr instruction) error {
	regX := instr.nibble(1)
	mask := instr.lower()
	m.regV[regX] = uint8(m.rng.UintN(256)) & mask
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

func (m *Machine) evalInstrF(instr instruction) error {
	regX := instr.nibble(1)
	switch instr.lower() {
	case 0x07:
		m.regV[regX] = m.delayTimer
	case 0x15:
		m.delayTimer = m.regV[regX]
	case 0x18:
		m.soundTimer = m.regV[regX]
	case 0x1e:
		if m.regI+uint16(m.regV[regX]) >= MemSize {
			return errors.New("set I past end of memory")
		}
		m.regI += uint16(m.regV[regX])
	case 0x29:
		ch := m.regV[regX] & 0x0f
		m.regI = BuiltinFontAddress + uint16(ch*BuiltinFontWidth)
	case 0x33:
		if m.regI+3 > MemSize {
			return errors.New("write past end of memory")
		}
		val := m.regV[regX]
		dig1 := uint8(val / 100)
		dig2 := uint8((val - (dig1 * 100)) / 10)
		dig3 := uint8((val - (dig1 * 100) - (dig2 * 10)))
		m.mem[m.regI] = dig1
		m.mem[m.regI+1] = dig2
		m.mem[m.regI+2] = dig3
	case 0x55:
		if m.regI+uint16(regX) >= MemSize {
			return errors.New("write past end of memory")
		}
		for i := range regX + 1 {
			m.mem[m.regI+uint16(i)] = m.regV[i]
		}
		m.regI += uint16(regX) + 1
	case 0x65:
		if m.regI+uint16(regX) >= MemSize {
			return errors.New("read past end of memory")
		}
		for i := range regX + 1 {
			m.regV[i] = m.mem[m.regI+uint16(i)]
		}
		m.regI += uint16(regX) + 1
	default:
		return fmt.Errorf("unknown instruction: %04x", instr.all())
	}
	return nil
}
