package gremul8

import "encoding/binary"

const (
	InstructionSize = 2
)

type InstrType int

const (
	InstrTypeNone InstrType = iota
	InstrTypeClear
	InstrTypeRet
	// TODO
)

const (
	InstrClear            uint16 = 0x00e0
	InstrReturn                  = 0x00ee
	InstrJump                    = 0x1000
	InstrCall                    = 0x2000
	InstrSkipEqualVxKk           = 0x3000
	InstrSkipNotSqualVxKk        = 0x4000
	InstrSkipEqualVxVy           = 0x5000
	InstrLoadVxKk                = 0x6000
	InstrAddVxKk                 = 0x7000
	InstrLoadVxVy                = 0x8000
	InstrOrVxVy                  = 0x8001
	InstrAndVxVy                 = 0x8002
	InstrXorVxVy                 = 0x8003
	InstrAddVxVy                 = 0x8004
	InstrSubVxVy                 = 0x8005
	InstrShiftRight              = 0x8006
	InstrSubNot                  = 0x8007
	InstrShiftLeft               = 0x800e
	InstrSkipNotEqualVxVy        = 0x9000
	InstrLoadI                   = 0xa000
	InstrJumpV0                  = 0xb000
	InstrRandVxKk                = 0xc000
	InstrDrawVxVy                = 0xd000
	InstrSkipKeyPress            = 0xe09e
	InstrSkipKeyNotPress         = 0xe0a1
	InstrLoadVxDelayTimer        = 0xf007
	InstrLoadKeyPress            = 0xf00a
	InstrLoadDelayTimerVx        = 0xf015
	InstrLoadSoundTimerVx        = 0xf018
	InstrAddIVx                  = 0xf01e
	InstrLoadISpriteFx           = 0xf029
	InstrLoadBcdVx               = 0xf033
	InstrLoadVxI                 = 0xf055
	InstrLoadIVx                 = 0xf065
	/*
		3.2 - Super Chip-48 Instructions           [TOC]

		00Cn - SCD nibble
		00FB - SCR
		00FC - SCL
		00FD - EXIT
		00FE - LOW
		00FF - HIGH
		Dxy0 - DRW Vx, Vy, 0
		Fx30 - LD HF, Vx
		Fx75 - LD R, Vx
		Fx85 - LD Vx, R
	*/
)

/*
type Instruction struct {
	Type InstructionType
	//Code [2]byte
}

func NewInstruction(data []byte) (Instruction, error) {
	if len(data) != 2 {
		return Instruction{}, errors.New("data must be 2 bytes")
	}
	tmp := binary.BigEndian.Uint16(data)
	switch {
	case tmp&InstrCls == InstrCls:
		return Instruction{Type: InstrTypeCls}
	}
}
*/

type instruction [2]byte

func (i instruction) nibble(idx int) uint8 {
	switch idx {
	case 0:
		return (i[0] & 0xf0) >> 4
	case 1:
		return i[0] & 0x0f
	case 2:
		return (i[1] & 0xf0) >> 4
	case 3:
		return i[1] & 0x0f
	default:
		panic("cannot access nibble above 3")
	}
}

func (i instruction) nibbles() [4]uint8 {
	return [4]uint8{(i[0] & 0xf0) >> 4, i[0] & 0x0f, (i[1] & 0xf0) >> 4, i[1] & 0x0f}
}

func (i instruction) address() uint16 {
	return i.all() & 0x0fff
}

func (i instruction) lower() uint8 {
	return uint8(i[1])
}

func (i instruction) all() uint16 {
	return binary.BigEndian.Uint16(i[:])
}
