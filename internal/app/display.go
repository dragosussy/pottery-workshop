package app

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"humidity/internal/hw"
)

// Screen colours: the web app's dark "tide" palette.
var (
	screenBG    = color.RGBA{0x0e, 0x17, 0x1c, 0xff}
	screenText  = color.RGBA{0xf1, 0xf5, 0xf7, 0xff}
	screenMuted = color.RGBA{0x8f, 0xa3, 0xad, 0xff}
	screenTeal  = color.RGBA{0x4f, 0xd1, 0xc5, 0xff}
	screenWarn  = color.RGBA{0xf3, 0xc4, 0x5a, 0xff}
)

// DisplayLoop keeps the screen next to the Pi showing the room's humidity
// and temperature, and the target (or that humidity control is off). It
// redraws when the shown values change (checked at each
// sensor read), and blanks the screen when ctx ends.
func (a *App) DisplayLoop(ctx context.Context, s hw.Screen) {
	defer s.Close()
	faces, err := newScreenFaces()
	if err != nil {
		log.Printf("display: %v", err)
		return
	}
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	shown := ""
	for {
		room, ok := a.Live.Room(time.Now())
		target := a.Live.Target()
		key := fmt.Sprintf("no reading %d", target)
		if ok {
			key = fmt.Sprintf("%.1f %.1f %d", room.RH, room.Temp, target)
		}
		if key != shown {
			if err := s.Show(renderScreen(faces, room, ok, target)); err != nil {
				log.Printf("display: %v", err)
			} else {
				shown = key
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

type screenFaces struct{ label, big, unit, temp font.Face }

func newScreenFaces() (screenFaces, error) {
	regular, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return screenFaces{}, err
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return screenFaces{}, err
	}
	face := func(f *opentype.Font, size float64) font.Face {
		fc, _ := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
		return fc
	}
	return screenFaces{
		label: face(regular, 20),
		big:   face(bold, 96),
		unit:  face(bold, 40),
		temp:  face(regular, 34),
	}, nil
}

// renderScreen draws one frame: a label and the target (0: control off) on
// top, the humidity in large type, and the temperature underneath.
func renderScreen(f screenFaces, room Room, ok bool, target int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, hw.ScreenWidth, hw.ScreenHeight))
	draw.Draw(img, img.Bounds(), image.NewUniform(screenBG), image.Point{}, draw.Src)
	text := func(face font.Face, c color.Color, x, y int, s string) int {
		d := font.Drawer{Dst: img, Src: image.NewUniform(c), Face: face, Dot: fixed.P(x, y)}
		d.DrawString(s)
		return d.Dot.X.Round()
	}

	text(f.label, screenMuted, 16, 30, "Humidity")
	right := "Control off"
	if target > 0 {
		right = fmt.Sprintf("Target %d %%", target)
	}
	text(f.label, screenMuted, hw.ScreenWidth-16-font.MeasureString(f.label, right).Round(), 30, right)
	if !ok {
		text(f.big, screenMuted, 14, 118, "–")
		text(f.temp, screenWarn, 16, 158, "no reading")
		return img
	}
	x := text(f.big, screenText, 12, 118, fmt.Sprintf("%.1f", room.RH))
	text(f.unit, screenTeal, x+6, 118, "%")
	text(f.temp, screenMuted, 16, 158, fmt.Sprintf("%.1f °C", room.Temp))
	return img
}
