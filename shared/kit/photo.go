package kit

import (
	"bytes"
	"image"
	"io"
	"os"

	"github.com/disintegration/imaging"
)

// MaxPhoto is the largest upload: a phone camera's original photo.
const MaxPhoto = 25 << 20

// ReadPhoto decodes an upload straight from a phone's camera (JPEG or PNG),
// turned upright from its EXIF orientation. It's done here rather than on the
// phone because browsers with fingerprinting protection scramble canvas output.
func ReadPhoto(body io.Reader) (image.Image, error) {
	data, err := io.ReadAll(io.LimitReader(body, MaxPhoto+1))
	if err != nil {
		return nil, Errorf(400, "reading the photo: %v", err)
	}
	if len(data) > MaxPhoto {
		return nil, Errorf(413, "the photo is over 25 MB")
	}
	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, Errorf(400, "the photo must be a JPEG or PNG")
	}
	return img, nil
}

// SavePhoto shrinks img to fit edge pixels and writes it to path as a JPEG.
// It writes then renames, so a reader never sees half a photo.
func SavePhoto(img image.Image, path string, edge int) error {
	var out bytes.Buffer
	fitted := imaging.Fit(img, edge, edge, imaging.Lanczos)
	if err := imaging.Encode(&out, fitted, imaging.JPEG, imaging.JPEGQuality(85)); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
