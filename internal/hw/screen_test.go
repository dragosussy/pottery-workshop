package hw

import (
	"image"
	"image/color"
	"testing"
)

func TestToRGB565(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 1))
	for i, c := range []color.RGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {255, 255, 255, 255}} {
		img.SetRGBA(i, 0, c)
	}
	out := make([]byte, 8)
	toRGB565(img, out)
	want := []byte{0xF8, 0x00, 0x07, 0xE0, 0x00, 0x1F, 0xFF, 0xFF}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("toRGB565 = % x, want % x", out, want)
		}
	}
}
