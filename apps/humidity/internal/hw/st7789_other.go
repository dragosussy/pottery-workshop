//go:build !linux

package hw

import (
	"errors"
	"image"
)

// ST7789 is unavailable off Linux; use SCREEN=sim on a laptop.
type ST7789 struct{}

func OpenST7789(spidev string, dcPin, rstPin, blPin int, flip bool) (*ST7789, error) {
	return nil, errors.New("the ST7789 screen requires Linux")
}

func (s *ST7789) Show(img *image.RGBA) error { return errors.New("the ST7789 screen requires Linux") }
func (s *ST7789) Close() error               { return nil }
