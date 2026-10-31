// Package photos stores piece photos: it re-encodes uploads (dropping any
// metadata), makes thumbnails, serves both, and deletes photos no piece
// refers to.
package photos

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/image/draw"

	"humidity/internal/store"
)

// Limits (backend plan §7).
const (
	MaxBytes  = 3 << 20
	maxPixels = 20_000_000
	thumbEdge = 320
	quality   = 85
	orphanAge = 24 * time.Hour
)

// Upload errors.
var (
	ErrNotJPEG    = errors.New("not a JPEG image")
	ErrTooManyPix = errors.New("image larger than 20 megapixels")
	ErrNotFound   = errors.New("photo not found")
)

// Service stores photos as PHOTO_DIR/<id>.jpg and <id>_thumb.jpg.
type Service struct {
	st  *store.Store
	dir string
	log *slog.Logger
	Now func() time.Time
}

// New returns a Service, creating dir if needed.
func New(st *store.Store, dir string, log *slog.Logger) (*Service, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Service{st: st, dir: dir, log: log, Now: time.Now}, nil
}

// Created is the POST /photos response.
type Created struct {
	ID        int64  `json:"id"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	CreatedAt string `json:"created_at"`
}

func (s *Service) path(id int64, thumb bool) string {
	if thumb {
		return filepath.Join(s.dir, fmt.Sprintf("%d_thumb.jpg", id))
	}
	return filepath.Join(s.dir, fmt.Sprintf("%d.jpg", id))
}

// Upload decodes a JPEG (the caller limits its size), re-encodes it and a
// thumbnail, and stores both.
func (s *Service) Upload(ctx context.Context, body []byte) (Created, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || format != "jpeg" {
		return Created{}, ErrNotJPEG
	}
	if cfg.Width*cfg.Height > maxPixels {
		return Created{}, ErrTooManyPix
	}
	img, err := jpeg.Decode(bytes.NewReader(body))
	if err != nil {
		return Created{}, ErrNotJPEG
	}

	var full, thumb bytes.Buffer
	if err := jpeg.Encode(&full, img, &jpeg.Options{Quality: quality}); err != nil {
		return Created{}, err
	}
	if err := jpeg.Encode(&thumb, scale(img, thumbEdge), &jpeg.Options{Quality: quality}); err != nil {
		return Created{}, err
	}

	b := img.Bounds()
	now := s.Now().UTC().Truncate(time.Second)
	p := store.Photo{Width: b.Dx(), Height: b.Dy(), Bytes: int64(full.Len()), CreatedAt: now}
	err = s.st.Tx(ctx, func(q store.Querier) error {
		id, err := store.InsertPhoto(ctx, q, p)
		if err != nil {
			return err
		}
		p.ID = id
		if err := writeFile(s.path(id, false), full.Bytes()); err != nil {
			return err
		}
		if err := writeFile(s.path(id, true), thumb.Bytes()); err != nil {
			os.Remove(s.path(id, false))
			return err
		}
		return nil
	})
	if err != nil {
		return Created{}, err
	}
	return Created{ID: p.ID, Width: p.Width, Height: p.Height, CreatedAt: now.Format(time.RFC3339)}, nil
}

// scale shrinks img so its long edge is at most edge pixels.
func scale(img image.Image, edge int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= edge && h <= edge {
		return img
	}
	if w >= h {
		h, w = max(h*edge/w, 1), edge
	} else {
		w, h = max(w*edge/h, 1), edge
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// writeFile writes to a temp file and renames it into place.
func writeFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Open returns the image file (full size, or the thumbnail).
func (s *Service) Open(ctx context.Context, id int64, thumb bool) (*os.File, store.Photo, error) {
	p, err := store.GetPhoto(ctx, s.st.DB(), id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, store.Photo{}, ErrNotFound
	}
	if err != nil {
		return nil, store.Photo{}, err
	}
	f, err := os.Open(s.path(id, thumb))
	if errors.Is(err, os.ErrNotExist) {
		return nil, store.Photo{}, ErrNotFound
	}
	return f, p, err
}

// Cleanup deletes photos older than 24 h that no piece refers to.
func (s *Service) Cleanup(ctx context.Context) (int, error) {
	ids, err := store.OrphanPhotos(ctx, s.st.DB(), s.Now().Add(-orphanAge))
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if err := store.DeletePhoto(ctx, s.st.DB(), id); err != nil {
			return 0, err
		}
		os.Remove(s.path(id, false))
		os.Remove(s.path(id, true))
	}
	return len(ids), nil
}

// RunCleanup runs Cleanup hourly until ctx is cancelled.
func (s *Service) RunCleanup(ctx context.Context) error {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if n, err := s.Cleanup(ctx); err != nil && ctx.Err() == nil {
			s.log.Warn("photo cleanup failed", "err", err)
		} else if n > 0 {
			s.log.Info("deleted unused photos", "count", n)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// ReadLimited reads at most limit bytes, reporting whether there was more.
func ReadLimited(r io.Reader, limit int64) ([]byte, bool, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(b)) > limit {
		return nil, true, nil
	}
	return b, false, nil
}
