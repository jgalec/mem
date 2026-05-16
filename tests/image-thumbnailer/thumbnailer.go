package thumbnailer

import (
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
)

type Format int

const (
	FormatJPEG Format = iota
	FormatPNG
)

type Config struct {
	MaxWidth  int
	MaxHeight int
	Quality   int
	Format    Format
}

type Thumbnailer struct {
	cfg Config
}

func New(cfg Config) *Thumbnailer {
	if cfg.Quality <= 0 || cfg.Quality > 100 {
		cfg.Quality = 85
	}
	return &Thumbnailer{cfg: cfg}
}

func (t *Thumbnailer) Generate(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer src.Close()

	img, format, err := image.Decode(src)
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	if srcW <= 0 || srcH <= 0 {
		return fmt.Errorf("invalid image dimensions: %dx%d", srcW, srcH)
	}

	newW, newH := t.calculateSize(srcW, srcH)

	var resized image.Image
	if newW == srcW && newH == srcH && t.cfg.Format == formatFromString(format) {
		resized = img
	} else if newW == srcW && newH == srcH {
		resized = img
	} else {
		resized = resizeBilinear(img, newW, newH)
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	out, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer out.Close()

	switch t.cfg.Format {
	case FormatPNG:
		return png.Encode(out, resized)
	default:
		return jpeg.Encode(out, resized, &jpeg.Options{Quality: t.cfg.Quality})
	}
}

func (t *Thumbnailer) calculateSize(srcW, srcH int) (int, int) {
	maxW := t.cfg.MaxWidth
	maxH := t.cfg.MaxHeight

	if maxW <= 0 && maxH <= 0 {
		return srcW, srcH
	}
	if maxW <= 0 {
		maxW = math.MaxInt32
	}
	if maxH <= 0 {
		maxH = math.MaxInt32
	}

	if srcW <= maxW && srcH <= maxH {
		return srcW, srcH
	}

	ratio := math.Min(float64(maxW)/float64(srcW), float64(maxH)/float64(srcH))
	newW := int(math.Round(float64(srcW) * ratio))
	newH := int(math.Round(float64(srcH) * ratio))

	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}

	return newW, newH
}

func resizeBilinear(src image.Image, dstW, dstH int) image.Image {
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))

	xRatio := float64(srcW-1) / float64(dstW)
	yRatio := float64(srcH-1) / float64(dstH)

	for y := range dstH {
		for x := range dstW {
			srcX := float64(x) * xRatio
			srcY := float64(y) * yRatio

			x0 := int(math.Floor(srcX))
			y0 := int(math.Floor(srcY))
			x1 := min(x0+1, srcW-1)
			y1 := min(y0+1, srcH-1)

			xFrac := srcX - float64(x0)
			yFrac := srcY - float64(y0)

			r00, g00, b00, a00 := src.At(srcBounds.Min.X+x0, srcBounds.Min.Y+y0).RGBA()
			r10, g10, b10, a10 := src.At(srcBounds.Min.X+x1, srcBounds.Min.Y+y0).RGBA()
			r01, g01, b01, a01 := src.At(srcBounds.Min.X+x0, srcBounds.Min.Y+y1).RGBA()
			r11, g11, b11, a11 := src.At(srcBounds.Min.X+x1, srcBounds.Min.Y+y1).RGBA()

			r := bilerp(r00, r10, r01, r11, xFrac, yFrac)
			g := bilerp(g00, g10, g01, g11, xFrac, yFrac)
			b := bilerp(b00, b10, b01, b11, xFrac, yFrac)
			a := bilerp(a00, a10, a01, a11, xFrac, yFrac)

			dst.Set(x, y, color.RGBA64{
				R: uint16(r >> 16),
				G: uint16(g >> 16),
				B: uint16(b >> 16),
				A: uint16(a >> 16),
			})
		}
	}

	return dst
}

func bilerp(c00, c10, c01, c11 uint32, xFrac, yFrac float64) uint32 {
	f00 := float64(c00)
	f10 := float64(c10)
	f01 := float64(c01)
	f11 := float64(c11)

	top := f00 + (f10-f00)*xFrac
	bot := f01 + (f11-f01)*xFrac
	return uint32(top + (bot-top)*yFrac)
}

func formatFromString(s string) Format {
	switch strings.ToLower(s) {
	case "png":
		return FormatPNG
	default:
		return FormatJPEG
	}
}

func init() {
	image.RegisterFormat("jpeg", "\xff\xd8", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("png", "\x89PNG\r\n\x1a\n", png.Decode, png.DecodeConfig)
	image.RegisterFormat("gif", "GIF8", gif.Decode, gif.DecodeConfig)
}
