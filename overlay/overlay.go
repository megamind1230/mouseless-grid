package overlay

import (
	"image"
	"image/png"
	"os"
)

type Window interface {
	Render(img *image.NRGBA) error
	Width() uint16
	Height() uint16
	SetOpacity(opacity float64)
	Hide()
	Close()
}

func SaveDebugImage(img *image.NRGBA, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
