package app

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"atelier/apps/humidity/internal/hw"
)

func TestRenderScreen(t *testing.T) {
	faces, err := newScreenFaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, ok := range []bool{true, false} {
		img := renderScreen(faces, Room{RH: 81.2, Temp: 18.6}, ok, 80)
		if b := img.Bounds(); b.Dx() != hw.ScreenWidth || b.Dy() != hw.ScreenHeight {
			t.Fatalf("frame is %v", b)
		}
		// Something was drawn where the big number goes.
		drawn := 0
		for x := 10; x < 160; x++ {
			if img.RGBAAt(x, 90) != screenBG {
				drawn++
			}
		}
		if drawn == 0 {
			t.Errorf("ok=%v: nothing drawn across the number's row", ok)
		}
	}
}

func TestDisplayLoopWritesAFrame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "screen.png")
	a := &App{Live: &Live{}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.DisplayLoop(ctx, hw.PNGScreen{Path: path}); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if f, err := os.Open(path); err == nil {
			cfg, err := png.DecodeConfig(f)
			f.Close()
			if err != nil || cfg.Width != hw.ScreenWidth || cfg.Height != hw.ScreenHeight {
				t.Fatalf("screen.png: %+v, %v", cfg, err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no frame written")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
}
