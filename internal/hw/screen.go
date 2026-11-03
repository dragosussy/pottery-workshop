package hw

import (
	"bytes"
	"image"
	"image/png"
	"os"
)

// toRGB565 packs a frame into the screen's 16-bit format, big-endian.
func toRGB565(img *image.RGBA, out []byte) {
	for i := 0; i < len(img.Pix)/4; i++ {
		r, g, b := img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2]
		v := uint16(r>>3)<<11 | uint16(g>>2)<<5 | uint16(b>>3)
		out[i*2], out[i*2+1] = byte(v>>8), byte(v)
	}
}

// PNGScreen is a fake screen that writes each frame to a PNG file, to see
// the layout on a laptop.
type PNGScreen struct{ Path string }

func (s PNGScreen) Show(img *image.RGBA) error {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, b.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

func (s PNGScreen) Close() error { return nil }
