package photos

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"humidity/internal/store"
)

func newService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	st.Migrate(context.Background())
	s, err := New(st, filepath.Join(dir, "photos"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return s, st
}

func testJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 120, 255})
		}
	}
	var b bytes.Buffer
	jpeg.Encode(&b, img, nil)
	return b.Bytes()
}

func TestUpload_ReencodesAndThumbnails(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	c, err := s.Upload(ctx, testJPEG(1600, 1200))
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != 1 || c.Width != 1600 || c.Height != 1200 {
		t.Errorf("created = %+v", c)
	}
	f, _, err := s.Open(ctx, c.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, err := jpeg.DecodeConfig(f)
	if err != nil || cfg.Width != 320 || cfg.Height != 240 {
		t.Errorf("thumbnail = %dx%d, %v; want 320x240", cfg.Width, cfg.Height, err)
	}
}

func TestUpload_Rejects(t *testing.T) {
	s, _ := newService(t)
	var p bytes.Buffer
	png.Encode(&p, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	for name, body := range map[string][]byte{"png": p.Bytes(), "garbage": []byte("hello"), "empty": nil} {
		if _, err := s.Upload(context.Background(), body); !errors.Is(err, ErrNotJPEG) {
			t.Errorf("%s: err = %v, want ErrNotJPEG", name, err)
		}
	}
	// A header claiming 6000×4000 (24 MP) is refused before decoding.
	big := testJPEG(6000, 4000)
	if _, err := s.Upload(context.Background(), big); !errors.Is(err, ErrTooManyPix) {
		t.Errorf("24 MP: err = %v, want ErrTooManyPix", err)
	}
}

func TestReadLimited(t *testing.T) {
	if _, over, _ := ReadLimited(bytes.NewReader(make([]byte, 11)), 10); !over {
		t.Error("11 bytes with limit 10 not reported as over")
	}
	if b, over, _ := ReadLimited(bytes.NewReader(make([]byte, 10)), 10); over || len(b) != 10 {
		t.Error("10 bytes with limit 10 refused")
	}
}

func TestCleanup_DeletesOldOrphansOnly(t *testing.T) {
	s, st := newService(t)
	ctx := context.Background()
	t0 := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return t0 }
	orphan, _ := s.Upload(ctx, testJPEG(10, 10))
	used, _ := s.Upload(ctx, testJPEG(10, 10))
	store.InsertPiece(ctx, st.DB(), store.Piece{Number: 1, Form: "mug", Size: "small", Thickness: "thin", AddedAt: t0, PhotoID: &used.ID})
	s.Now = func() time.Time { return t0.Add(time.Hour) }
	fresh, _ := s.Upload(ctx, testJPEG(10, 10))

	s.Now = func() time.Time { return t0.Add(25 * time.Hour) }
	n, err := s.Cleanup(ctx)
	if err != nil || n != 1 {
		t.Fatalf("Cleanup = %d, %v; want 1", n, err)
	}
	if _, _, err := s.Open(ctx, orphan.ID, false); !errors.Is(err, ErrNotFound) {
		t.Error("orphan still there")
	}
	if _, err := os.Stat(s.path(orphan.ID, true)); !os.IsNotExist(err) {
		t.Error("orphan thumbnail file still there")
	}
	for _, id := range []int64{used.ID, fresh.ID} {
		if f, _, err := s.Open(ctx, id, false); err != nil {
			t.Errorf("photo %d deleted: %v", id, err)
		} else {
			f.Close()
		}
	}
}
