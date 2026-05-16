package thumbnailer

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func createTestPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 7) % 256),
				G: uint8((y * 13) % 256),
				B: uint8(128),
				A: 255,
			})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func createTestJPEG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 7) % 256),
				G: uint8((y * 13) % 256),
				B: uint8(128),
				A: 255,
			})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
}

func readImageSize(t *testing.T, path string) (int, int) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Width, cfg.Height
}

func TestGenerate_ResizeLargerToThumbnail(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.png")
	dst := filepath.Join(dir, "thumb.jpg")
	createTestPNG(t, src, 800, 600)

	tb := New(Config{MaxWidth: 200, MaxHeight: 150, Quality: 90})
	if err := tb.Generate(src, dst); err != nil {
		t.Fatal(err)
	}

	w, h := readImageSize(t, dst)
	if w != 200 || h != 150 {
		t.Errorf("expected 200x150, got %dx%d", w, h)
	}
}

func TestGenerate_AspectRatioPreserved(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.png")
	dst := filepath.Join(dir, "thumb.jpg")
	createTestPNG(t, src, 1600, 900)

	tb := New(Config{MaxWidth: 320, MaxHeight: 320, Quality: 85})
	if err := tb.Generate(src, dst); err != nil {
		t.Fatal(err)
	}

	w, h := readImageSize(t, dst)
	if w != 320 || h != 180 {
		t.Errorf("expected 320x180 (16:9 in 320x320 box), got %dx%d", w, h)
	}
}

func TestGenerate_NoUpscale(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.png")
	dst := filepath.Join(dir, "thumb.jpg")
	createTestPNG(t, src, 100, 80)

	tb := New(Config{MaxWidth: 500, MaxHeight: 500, Quality: 85})
	if err := tb.Generate(src, dst); err != nil {
		t.Fatal(err)
	}

	w, h := readImageSize(t, dst)
	if w != 100 || h != 80 {
		t.Errorf("expected no upscale (100x80), got %dx%d", w, h)
	}
}

func TestGenerate_JPEGToPNG(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.jpg")
	dst := filepath.Join(dir, "thumb.png")
	createTestJPEG(t, src, 400, 300)

	tb := New(Config{MaxWidth: 100, MaxHeight: 100, Format: FormatPNG})
	if err := tb.Generate(src, dst); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(dst); err != nil {
		t.Fatal("output file not created")
	}
	w, h := readImageSize(t, dst)
	if w != 100 || h != 75 {
		t.Errorf("expected 100x75, got %dx%d", w, h)
	}
}

func TestGenerate_TinyImage(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.png")
	dst := filepath.Join(dir, "thumb.jpg")
	createTestPNG(t, src, 1, 1)

	tb := New(Config{MaxWidth: 100, MaxHeight: 100, Quality: 85})
	if err := tb.Generate(src, dst); err != nil {
		t.Fatal(err)
	}

	w, h := readImageSize(t, dst)
	if w != 1 || h != 1 {
		t.Errorf("expected 1x1, got %dx%d", w, h)
	}
}

func TestGenerate_OneDimOnly(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.png")
	dst := filepath.Join(dir, "thumb_w.jpg")
	createTestPNG(t, src, 800, 600)

	tb := New(Config{MaxWidth: 400, Quality: 85})
	if err := tb.Generate(src, dst); err != nil {
		t.Fatal(err)
	}

	w, h := readImageSize(t, dst)
	if w != 400 {
		t.Errorf("expected width 400, got %d", w)
	}
	if h != 300 {
		t.Errorf("expected height 300, got %d", h)
	}
}

func TestGenerate_MissingSource(t *testing.T) {
	tb := New(Config{MaxWidth: 100, MaxHeight: 100})
	err := tb.Generate("nonexistent.png", "out.jpg")
	if err == nil {
		t.Fatal("expected error for missing source")
	}
}
