package gremul8

import (
	"strings"
	"testing"
)

func TestDrawSpriteBitOrder(t *testing.T) {
	d := NewDummyDisplay(64, 32)
	// The high bit is the leftmost pixel
	if d.DrawSprite(0, 0, []byte{0b10110000}) {
		t.Errorf("unexpected collision on empty display")
	}
	if got, want := rowString(d, 0)[:8], "#.##...."; got != want {
		t.Errorf("row 0 = %q, want %q", got, want)
	}
}

func TestDrawSpriteTwiceErasesAndCollides(t *testing.T) {
	d := NewDummyDisplay(64, 32)
	sprite := []byte{0xf0, 0x90}
	if d.DrawSprite(4, 2, sprite) {
		t.Errorf("first draw: unexpected collision")
	}
	if !d.DrawSprite(4, 2, sprite) {
		t.Errorf("second draw: expected collision")
	}
	if n := litPixels(d); n != 0 {
		t.Errorf("%d pixels lit after drawing the sprite twice, want 0", n)
	}
}

// Zero bits in a sprite must leave the pixels underneath alone, and 1 bits must
// flip them.
func TestDrawSpriteZeroBitsLeavePixelsAlone(t *testing.T) {
	d := NewDummyDisplay(64, 32)
	d.DrawSprite(0, 0, []byte{0xff})
	if !d.DrawSprite(0, 0, []byte{0b10000001}) {
		t.Errorf("expected collision")
	}
	if got, want := rowString(d, 0)[:8], ".######."; got != want {
		t.Errorf("row 0 = %q, want %q", got, want)
	}
}

func TestDrawSpriteNoCollisionOnAdjacentSprites(t *testing.T) {
	d := NewDummyDisplay(64, 32)
	d.DrawSprite(0, 0, []byte{0xff})
	if d.DrawSprite(8, 0, []byte{0xff}) {
		t.Errorf("unexpected collision for adjacent sprite")
	}
	if d.DrawSprite(0, 1, []byte{0xff}) {
		t.Errorf("unexpected collision for sprite on next row")
	}
}

func TestDummyDisplaySize(t *testing.T) {
	for _, size := range [][2]uint8{{64, 32}, {128, 64}, {8, 4}} {
		d := NewDummyDisplay(size[0], size[1])
		if w, h := d.Size(); w != size[0] || h != size[1] {
			t.Errorf("Size() = %dx%d, want %dx%d", w, h, size[0], size[1])
		}
	}
}

func TestDummyDisplayStartsBlank(t *testing.T) {
	d := NewDummyDisplay(64, 32)
	if n := litPixels(d); n != 0 {
		t.Errorf("%d pixels lit on a new display, want 0", n)
	}
}

func TestDummyDisplayClear(t *testing.T) {
	d := NewDummyDisplay(64, 32)
	d.DrawSprite(0, 0, []byte{0xff, 0xff})
	d.DrawSprite(56, 30, []byte{0xff})
	if litPixels(d) == 0 {
		t.Fatalf("nothing drawn before the clear")
	}
	d.Clear()
	if n := litPixels(d); n != 0 {
		t.Errorf("%d pixels lit after Clear, want 0", n)
	}
	if w, h := d.Size(); w != 64 || h != 32 {
		t.Errorf("Size() after Clear = %dx%d, want 64x32", w, h)
	}
	// Drawing the same sprite again must not collide with what was cleared
	if d.DrawSprite(0, 0, []byte{0xff}) {
		t.Errorf("collision after Clear")
	}
}

// Sprites that exactly touch each corner of the largest display SUPER-CHIP
// needs must land on the right pixels.
func TestDummyDisplayHiresCorners(t *testing.T) {
	d := NewDummyDisplay(128, 64)
	// 8 wide, 2 tall; the corners of each sprite are lit, so the pixel at the
	// display corner is lit too.
	sprite := []byte{0x81, 0x81}
	d.DrawSprite(0, 0, sprite)
	d.DrawSprite(120, 0, sprite)
	d.DrawSprite(0, 62, sprite)
	d.DrawSprite(120, 62, sprite)
	for _, y := range []uint8{0, 1, 62, 63} {
		row := rowString(d, y)
		if got, want := row[:8], "#......#"; got != want {
			t.Errorf("row %d left = %q, want %q", y, got, want)
		}
		if got, want := row[120:], "#......#"; got != want {
			t.Errorf("row %d right = %q, want %q", y, got, want)
		}
		if mid := row[8:120]; strings.Contains(mid, "#") {
			t.Errorf("row %d has stray pixels in the middle: %q", y, mid)
		}
	}
	if n := litPixels(d); n != 16 {
		t.Errorf("%d pixels lit, want 16", n)
	}
}
