# Image Thumbnailer PRD

## Overview

A Go package for generating image thumbnails from source image files. Reads common image formats (JPEG, PNG, GIF) and produces scaled-down thumbnails while preserving aspect ratio. Designed as a test utility to validate that the memory system correctly handles image processing workflows and artifact generation.

## Requirements

- **Format support**: Decode JPEG, PNG, and GIF (static frames only)
- **Output formats**: JPEG (default) and PNG
- **Aspect ratio preservation**: Maintain original proportions when scaling
- **Configurable dimensions**: Target `MaxWidth` and `MaxHeight`; thumbnail fits within bounds
- **Quality control**: Configurable JPEG quality (1-100, default 85)
- **In-place scaling**: Skip resize if source is already smaller than target dimensions
- **Error handling**: Return descriptive errors for unsupported formats or corrupt files

## Files

| File | Purpose |
|------|---------|
| `thumbnailer.go` | Core `Thumbnailer` type, `Config`, thumbnail generation logic |
| `thumbnailer_test.go` | Unit tests for resize, format conversion, aspect ratio, edge cases |

## API

```go
type Config struct {
    MaxWidth  int    // Max thumbnail width in pixels (0 = use MaxHeight only)
    MaxHeight int    // Max thumbnail height in pixels (0 = use MaxWidth only)
    Quality   int    // JPEG quality 1-100 (default 85)
    Format    Format // Output format: FormatJPEG or FormatPNG
}

type Format int

const (
    FormatJPEG Format = iota
    FormatPNG
)

func New(cfg Config) *Thumbnailer
func (t *Thumbnailer) Generate(srcPath, dstPath string) error
```

## Behavior

1. Decode source image from `srcPath`
2. If image dimensions are already within `MaxWidth` x `MaxHeight`, copy unchanged (skip resize)
3. Calculate new dimensions preserving aspect ratio to fit within bounds
4. Create a new image at target dimensions using bilinear interpolation
5. Encode and write to `dstPath` in the configured output format

## Decisions

- Bilinear interpolation over nearest-neighbor for better visual quality with minimal perf cost
- Copy unchanged when source fits within bounds instead of upscaling
- JPEG defaults over PNG for smaller file size in typical thumbnail use cases

## Lessons

- Always validate image dimensions before allocating resize buffers to avoid OOM on pathological inputs
- Aspect ratio math requires float division then rounding; integer-only arithmetic causes off-by-one drift
