package phash

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// gradient builds a deterministic non-flat test image.
func gradient(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8((x*255)/w ^ (y * 3))
			img.Set(x, y, color.RGBA{v, uint8((y * 255) / h), 128, 255})
		}
	}
	return img
}

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestSameImageDifferentEncodingIsClose(t *testing.T) {
	img := gradient(64, 64)
	hPng, err := Compute(pngBytes(t, img))
	if err != nil {
		t.Fatalf("png: %v", err)
	}
	hJpg, err := Compute(jpegBytes(t, img))
	if err != nil {
		t.Fatalf("jpeg: %v", err)
	}
	if d := Distance(hPng, hJpg); d > 4 {
		t.Errorf("same image across encodings distance = %d, want small", d)
	}
}

func TestDifferentImagesAreFar(t *testing.T) {
	a, _ := Compute(pngBytes(t, gradient(64, 64)))
	// A very different image: inverted gradient.
	inv := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			inv.Set(x, y, color.RGBA{uint8(255 - x*4), uint8(255 - y*4), 200, 255})
		}
	}
	b, _ := Compute(pngBytes(t, inv))
	if d := Distance(a, b); d < 8 {
		t.Errorf("different images distance = %d, want large", d)
	}
}

func TestDecodeErrorReported(t *testing.T) {
	if _, err := Compute([]byte("not an image")); err == nil {
		t.Fatal("expected decode error")
	}
}
