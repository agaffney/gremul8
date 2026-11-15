package gremul8

import "testing"

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
