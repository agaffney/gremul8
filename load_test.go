package gremul8

import (
	"bytes"
	"errors"
	"testing"
	"testing/iotest"
)

// recordingDisplay wraps a DummyDisplay and records how the machine uses it.
type recordingDisplay struct {
	*DummyDisplay
	clears int
	draws  int
}

func (d *recordingDisplay) Clear() {
	d.clears++
	d.DummyDisplay.Clear()
}

func (d *recordingDisplay) DrawSprite(x, y uint8, data []byte) bool {
	d.draws++
	return d.DummyDisplay.DrawSprite(x, y, data)
}

func TestNewMachineUsesGivenDisplay(t *testing.T) {
	d := &recordingDisplay{DummyDisplay: NewDummyDisplay(64, 32)}
	m := NewMachine(d)
	if d.clears == 0 {
		t.Errorf("NewMachine did not clear the display it was given")
	}
	// LD I, font "0"; DRW V0, V0, 5
	if err := m.Load(bytes.NewReader([]byte{0xa0, 0x50, 0xd0, 0x05})); err != nil {
		t.Fatal(err)
	}
	step(t, m)
	step(t, m)
	if d.draws != 1 {
		t.Errorf("display saw %d draws, want 1", d.draws)
	}
	if litPixels(d) == 0 {
		t.Errorf("sprite did not reach the given display")
	}
}

func TestLoadClearsGivenDisplay(t *testing.T) {
	d := &recordingDisplay{DummyDisplay: NewDummyDisplay(64, 32)}
	m := NewMachine(d)
	d.DrawSprite(0, 0, []byte{0xff})
	before := d.clears
	if err := m.Load(bytes.NewReader([]byte{0x12, 0x00})); err != nil {
		t.Fatal(err)
	}
	if d.clears != before+1 {
		t.Errorf("Load cleared the display %d times, want 1", d.clears-before)
	}
	if n := litPixels(d); n != 0 {
		t.Errorf("%d pixels lit after Load, want 0", n)
	}
}

func TestNewMachineNilDisplayDefaults(t *testing.T) {
	m := NewMachine(nil)
	if m.disp == nil {
		t.Fatalf("no display created")
	}
	if w, h := m.disp.Size(); w != 64 || h != 32 {
		t.Errorf("default display is %dx%d, want 64x32", w, h)
	}
}

func TestLoadReadError(t *testing.T) {
	m := NewMachine(nil)
	boom := errors.New("boom")
	if err := m.Load(iotest.ErrReader(boom)); !errors.Is(err, boom) {
		t.Errorf("Load error = %v, want %v", err, boom)
	}
}

func TestLoadReadErrorAfterData(t *testing.T) {
	m := NewMachine(nil)
	boom := errors.New("boom")
	r := &errAfterReader{data: []byte{0x12, 0x00}, err: boom}
	if err := m.Load(r); !errors.Is(err, boom) {
		t.Errorf("Load error = %v, want %v", err, boom)
	}
}

type errAfterReader struct {
	data []byte
	err  error
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func TestLoadEmptyProgram(t *testing.T) {
	m := newMachineWithROM(t, nil)
	if m.pc != RomAddress {
		t.Errorf("pc = %04x, want %04x", m.pc, RomAddress)
	}
	// Memory is zeroed, so the first instruction is 0000, which is unknown
	ok, err := stepNoPanic(t, m)
	if ok {
		wantErrContaining(t, err, "unknown instruction")
	}
	if !fontIntact(m) {
		t.Errorf("font missing after loading an empty program")
	}
}
