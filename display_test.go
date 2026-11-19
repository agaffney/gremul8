package gremul8

import (
	"fmt"
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

// Pixels that fall past the right or bottom edge are clipped, not wrapped and not
// a panic. The boundary cases matter most: a pixel at x == width (or y == height)
// is already off the display.
func TestDrawSpriteClipsAtRightEdge(t *testing.T) {
	for _, size := range [][2]uint8{{64, 32}, {128, 64}, {8, 4}} {
		w, h := size[0], size[1]
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			d := NewDummyDisplay(w, h)
			var ok bool
			var collided bool

			// Ends exactly on the last column: nothing is clipped
			ok = noPanic(t, "draw ending at last column", func() { d.DrawSprite(w-8, 0, []byte{0xff}) })
			if !ok {
				return
			}
			if got, want := rowString(d, 0)[w-8:], "########"; got != want {
				t.Errorf("last 8 columns = %q, want %q", got, want)
			}
			d.Clear()

			// One column too far: the last pixel is off the display
			ok = noPanic(t, "draw one column past the edge", func() { collided = d.DrawSprite(w-7, 0, []byte{0xff}) })
			if !ok {
				return
			}
			if collided {
				t.Errorf("collision reported for a pixel that is off the display")
			}
			if got, want := rowString(d, 0)[w-7:], "#######"; got != want {
				t.Errorf("last 7 columns = %q, want %q", got, want)
			}
			if n := litPixels(d); n != 7 {
				t.Errorf("%d pixels lit, want 7 (the 8th must be clipped, not wrapped)", n)
			}
			d.Clear()

			// Only the leftmost pixel is on the display
			if !noPanic(t, "draw with one visible column", func() { d.DrawSprite(w-1, 0, []byte{0xff}) }) {
				return
			}
			if got, want := rowString(d, 0)[w-1:], "#"; got != want {
				t.Errorf("last column = %q, want %q", got, want)
			}
			if n := litPixels(d); n != 1 {
				t.Errorf("%d pixels lit, want 1", n)
			}
			d.Clear()

			// Starting exactly at the width: the start position wraps to column 0
			// (the sprite is not clipped away)
			if !noPanic(t, "draw starting at x == width", func() { d.DrawSprite(w, 0, []byte{0xff}) }) {
				return
			}
			if got, want := rowString(d, 0)[:8], "########"; got != want {
				t.Errorf("first 8 columns = %q, want %q (start position wraps)", got, want)
			}
			if n := litPixels(d); n != 8 {
				t.Errorf("%d pixels lit, want 8", n)
			}
		})
	}
}

func TestDrawSpriteClipsAtBottomEdge(t *testing.T) {
	for _, size := range [][2]uint8{{64, 32}, {128, 64}, {8, 4}} {
		w, h := size[0], size[1]
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			d := NewDummyDisplay(w, h)
			sprite := []byte{0x80, 0x80, 0x80}

			// Ends exactly on the last row: nothing is clipped
			if !noPanic(t, "draw ending at last row", func() { d.DrawSprite(0, h-3, sprite) }) {
				return
			}
			if n := litPixels(d); n != 3 {
				t.Errorf("%d pixels lit, want 3", n)
			}
			d.Clear()

			// One row too far: the last row is off the display
			var collided bool
			if !noPanic(t, "draw one row past the edge", func() { collided = d.DrawSprite(0, h-2, sprite) }) {
				return
			}
			if collided {
				t.Errorf("collision reported for a row that is off the display")
			}
			if n := litPixels(d); n != 2 {
				t.Errorf("%d pixels lit, want 2 (the 3rd row must be clipped, not wrapped)", n)
			}
			if d.IsSet(0, 0) {
				t.Errorf("clipped row wrapped to the top of the display")
			}
			d.Clear()

			// Starting exactly at the height: the start position wraps to row 0
			if !noPanic(t, "draw starting at y == height", func() { d.DrawSprite(0, h, sprite) }) {
				return
			}
			for y := range uint8(3) {
				if !d.IsSet(0, y) {
					t.Errorf("pixel (0,%d) not lit, want the start position to wrap to row 0", y)
				}
			}
			if n := litPixels(d); n != 3 {
				t.Errorf("%d pixels lit, want 3", n)
			}
		})
	}
}

func TestDrawSpriteClipsAtCorner(t *testing.T) {
	d := NewDummyDisplay(64, 32)
	if !noPanic(t, "draw past the corner", func() { d.DrawSprite(60, 30, []byte{0xff, 0xff, 0xff, 0xff}) }) {
		return
	}
	// 4 columns by 2 rows survive
	if n := litPixels(d); n != 8 {
		t.Errorf("%d pixels lit, want 8\n%s", n, renderDisplay(d))
	}
}

// A collision with a pixel that would be off the display cannot happen, so
// clipped pixels must not influence the collision flag.
func TestDrawSpriteClippedPixelsDoNotCollide(t *testing.T) {
	d := NewDummyDisplay(64, 32)
	d.DrawSprite(0, 0, []byte{0xff}) // lit at the left edge, where a wrapped pixel would land
	if !noPanic(t, "draw past the right edge", func() {
		if d.DrawSprite(60, 0, []byte{0xff}) {
			t.Errorf("clipped pixels collided with pixels on the opposite edge")
		}
	}) {
		return
	}
	if got, want := rowString(d, 0)[:8], "########"; got != want {
		t.Errorf("left edge was altered by a clipped sprite: %q", got)
	}
}

// The start position wraps around the display (x % width, y % height); only the
// pixels that then run off the right or bottom edge are clipped. Each case lists
// the rectangle that must be lit, which is where the 8x2 sprite lands after
// wrapping, cut off at the display edge.
func TestDrawSpriteStartPositionWraps(t *testing.T) {
	tests := []struct {
		x, y           uint8
		x0, x1, y0, y1 int // lit pixels are exactly [x0,x1) x [y0,y1)
	}{
		{0, 0, 0, 8, 0, 2},
		{64, 0, 0, 8, 0, 2},    // x == width
		{100, 0, 36, 44, 0, 2}, // 100 - 64
		{250, 0, 58, 64, 0, 2}, // 250 - 3*64 = 58, so 2 columns are clipped
		{255, 0, 63, 64, 0, 2}, // 255 - 3*64 = 63, only one column visible
		{0, 32, 0, 8, 0, 2},    // y == height
		{0, 100, 0, 8, 4, 6},   // 100 - 3*32
		{0, 250, 0, 8, 26, 28}, // 250 - 7*32
		{0, 255, 0, 8, 31, 32}, // 255 - 7*32 = 31, second row clipped
		{250, 250, 58, 64, 26, 28},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d,%d", tt.x, tt.y), func(t *testing.T) {
			d := NewDummyDisplay(64, 32)
			var collided bool
			if !noPanic(t, "draw", func() { collided = d.DrawSprite(tt.x, tt.y, []byte{0xff, 0xff}) }) {
				return
			}
			if collided {
				t.Errorf("collision reported on an empty display")
			}
			for y := range uint8(32) {
				for x := range uint8(64) {
					want := int(x) >= tt.x0 && int(x) < tt.x1 && int(y) >= tt.y0 && int(y) < tt.y1
					if d.IsSet(x, y) != want {
						t.Errorf("pixel (%d,%d) set = %v, want %v\n%s", x, y, !want, want, renderDisplay(d))
						return
					}
				}
			}
		})
	}
}

// Wrapping must use the width for x and the height for y, whatever the size.
func TestDrawSpriteStartPositionWrapsHires(t *testing.T) {
	d := NewDummyDisplay(128, 64)
	d.DrawSprite(130, 66, []byte{0x80}) // wraps to (2, 2)
	if !d.IsSet(2, 2) || litPixels(d) != 1 {
		t.Errorf("want exactly pixel (2,2) lit\n%s", renderDisplay(d))
	}
	d.Clear()
	d.DrawSprite(100, 40, []byte{0x80}) // already on the display: unchanged
	if !d.IsSet(100, 40) || litPixels(d) != 1 {
		t.Errorf("want exactly pixel (100,40) lit")
	}
}
