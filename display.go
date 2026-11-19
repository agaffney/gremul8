package gremul8

type Display interface {
	Size() (uint8, uint8)
	Clear()
	IsSet(uint8, uint8) bool
	DrawSprite(x uint8, y uint8, data []byte) bool
}

type DummyDisplay struct {
	data  [][]byte
	sizeX uint8
	sizeY uint8
}

func NewDummyDisplay(sizeX uint8, sizeY uint8) *DummyDisplay {
	d := &DummyDisplay{
		sizeX: sizeX,
		sizeY: sizeY,
	}
	d.Clear()
	return d
}

func (d *DummyDisplay) Size() (uint8, uint8) {
	return d.sizeX, d.sizeY
}

func (d *DummyDisplay) Clear() {
	d.data = make([][]byte, d.sizeX)
	for i := range d.sizeX {
		d.data[i] = make([]byte, d.sizeY)
	}
}

func (d *DummyDisplay) IsSet(x uint8, y uint8) bool {
	return d.data[x][y] > 0
}

func (d *DummyDisplay) DrawSprite(x uint8, y uint8, sprite []byte) bool {
	ret := false
	// Wrap start position for screen size
	x = x % d.sizeX
	y = y % d.sizeY
	var pixel byte
	var tmpX uint8
	var tmpY uint8
	for i := range len(sprite) {
		tmp := sprite[i]
		tmpY = y + uint8(i)
		// Truncate past end of screen
		if tmpY >= d.sizeY {
			continue
		}
		for j := range 8 {
			pixel = min(tmp&(0x80>>j), 1)
			if pixel == 0 {
				continue
			}
			tmpX = x + uint8(j)
			// Truncate past end of screen
			if tmpX >= d.sizeX {
				continue
			}
			if d.data[tmpX][tmpY] > 0 {
				// Flip the pixel and set the collision flag
				pixel = 0
				ret = true
			}
			d.data[tmpX][tmpY] = pixel
		}
	}
	return ret
}
