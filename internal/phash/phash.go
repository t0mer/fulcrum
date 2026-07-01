// Package phash computes a perceptual hash (dHash) of an image so near-identical
// copies (re-encoded, resized, lightly recompressed) can be recognized as
// duplicates even when their bytes differ. See CLAUDE.md §10.
package phash

import (
	"bytes"
	"fmt"
	"image"
	"math/bits"

	_ "image/jpeg" // register decoders
	_ "image/png"

	_ "golang.org/x/image/webp"
)

// hashW/hashH define the sampled grid. dHash compares horizontally adjacent
// pixels, yielding (hashW-1) * hashH bits — 8*8 = 64 here.
const (
	hashW = 9
	hashH = 8
)

// Compute returns the 64-bit dHash of an encoded image.
func Compute(data []byte) (uint64, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, fmt.Errorf("decoding image: %w", err)
	}
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw == 0 || sh == 0 {
		return 0, fmt.Errorf("empty image")
	}

	// Nearest-neighbour downscale to a small grayscale grid.
	var gray [hashH][hashW]float64
	for y := 0; y < hashH; y++ {
		for x := 0; x < hashW; x++ {
			sx := b.Min.X + x*sw/hashW
			sy := b.Min.Y + y*sh/hashH
			r, g, bl, _ := img.At(sx, sy).RGBA() // 16-bit per channel
			gray[y][x] = 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bl>>8)
		}
	}

	var hash uint64
	bit := 0
	for y := 0; y < hashH; y++ {
		for x := 0; x < hashW-1; x++ {
			if gray[y][x] > gray[y][x+1] {
				hash |= 1 << uint(bit)
			}
			bit++
		}
	}
	return hash, nil
}

// Distance returns the Hamming distance between two hashes (0 = identical).
func Distance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}
