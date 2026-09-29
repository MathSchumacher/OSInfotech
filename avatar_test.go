package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestMakeAvatarWebP(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 640, 480))
	for y := 0; y < 480; y++ {
		for x := 0; x < 640; x++ {
			src.Set(x, y, color.NRGBA{R: uint8(x % 255), G: uint8(y % 255), B: 120, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}

	webpBytes, preview, err := makeAvatarWebP(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(webpBytes) == 0 {
		t.Fatal("expected non-empty webp output")
	}
	if preview.Bounds().Dx() != 512 || preview.Bounds().Dy() != 512 {
		t.Fatalf("preview size = %v, want 512x512", preview.Bounds())
	}
}
