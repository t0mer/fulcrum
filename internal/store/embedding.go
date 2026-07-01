package store

import (
	"encoding/binary"
	"fmt"
	"math"
)

// EncodeEmbedding packs a float32 slice into a little-endian byte blob for
// storage. Embeddings are expected L2-normalized (the ML sidecar guarantees
// this) so the matcher can dot-product them directly.
func EncodeEmbedding(e []float32) []byte {
	b := make([]byte, len(e)*4)
	for i, f := range e {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// DecodeEmbedding reverses EncodeEmbedding.
func DecodeEmbedding(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("embedding blob length %d is not a multiple of 4", len(b))
	}
	e := make([]float32, len(b)/4)
	for i := range e {
		e[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return e, nil
}
