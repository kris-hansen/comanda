package quantizer

import (
	"fmt"
	"math"
)

// Config controls quantization behavior.
type Config struct {
	Bits int   // Bit width per coordinate: 1, 2, 3, or 4. Default: 2.
	Seed int64 // Random rotation seed. Default: 42.
}

// DefaultConfig returns sensible defaults (2-bit quantization, seed 42).
func DefaultConfig() Config {
	return Config{Bits: 2, Seed: 42}
}

// QuantizedVector is the compact representation of a quantized vector.
// At 2 bits per dimension, a 384-dim vector compresses from 3072 bytes
// (float64) to 96 bytes — a 32x reduction.
type QuantizedVector struct {
	Dim   int     // Original dimension
	Bits  int     // Bit width used
	Seed  int64   // Rotation seed (needed for dequantization)
	Codes []byte  // Packed quantization codes
	Norm  float64 // Original L2 norm (stored for cosine similarity)
}

// Quantize applies TurboQuant-style quantization: normalize → rotate →
// coordinate-wise Lloyd-Max quantization → pack codes.
func Quantize(vec []float64, cfg Config) (*QuantizedVector, error) {
	if len(vec) == 0 {
		return nil, fmt.Errorf("cannot quantize empty vector")
	}
	if cfg.Bits < 1 || cfg.Bits > 4 {
		return nil, fmt.Errorf("bit width must be 1-4, got %d", cfg.Bits)
	}

	dim := len(vec)
	cb, err := GetCodebook(cfg.Bits)
	if err != nil {
		return nil, err
	}

	// Compute L2 norm
	norm := 0.0
	for _, v := range vec {
		norm += v * v
	}
	norm = math.Sqrt(norm)

	if norm == 0 {
		// Zero vector: all codes are the center index closest to 0
		codes := packZeroCodes(dim, cb)
		return &QuantizedVector{
			Dim:   dim,
			Bits:  cfg.Bits,
			Seed:  cfg.Seed,
			Codes: codes,
			Norm:  0,
		}, nil
	}

	// Normalize to unit vector, then scale by sqrt(d) so coordinates
	// are approximately N(0,1) after rotation (per TurboQuant theory).
	sqrtD := math.Sqrt(float64(dim))
	rotated := make([]float64, dim)
	for i, v := range vec {
		rotated[i] = (v / norm) * sqrtD
	}

	// Apply random rotation
	rot := NewRotation(dim, cfg.Seed)
	rot.Apply(rotated)

	// Coordinate-wise quantization
	indices := make([]uint8, dim)
	for i, v := range rotated {
		indices[i] = cb.Quantize(v)
	}

	// Pack indices into bytes
	codes := packCodes(indices, cfg.Bits)

	return &QuantizedVector{
		Dim:   dim,
		Bits:  cfg.Bits,
		Seed:  cfg.Seed,
		Codes: codes,
		Norm:  norm,
	}, nil
}

// Dequantize reconstructs an approximate vector from its quantized representation.
func Dequantize(qv *QuantizedVector) ([]float64, error) {
	if qv == nil {
		return nil, fmt.Errorf("cannot dequantize nil vector")
	}

	cb, err := GetCodebook(qv.Bits)
	if err != nil {
		return nil, err
	}

	// Unpack indices
	indices := unpackCodes(qv.Codes, qv.Bits, qv.Dim)

	// Dequantize coordinates
	result := make([]float64, qv.Dim)
	for i, idx := range indices {
		result[i] = cb.Dequantize(idx)
	}

	// Inverse rotation
	rot := NewRotation(qv.Dim, qv.Seed)
	rot.ApplyInverse(result)

	// Undo normalization: scale from unit sphere back to original norm
	sqrtD := math.Sqrt(float64(qv.Dim))
	if sqrtD > 0 {
		for i := range result {
			result[i] = (result[i] / sqrtD) * qv.Norm
		}
	}

	return result, nil
}

// InnerProduct estimates the inner product <a, b> from two quantized vectors
// using the MSE estimator. Both vectors must have the same dimension and seed.
func InnerProduct(a, b *QuantizedVector) (float64, error) {
	if err := checkCompatible(a, b); err != nil {
		return 0, err
	}

	cb, err := GetCodebook(a.Bits)
	if err != nil {
		return 0, err
	}

	// Unpack and compute dot product in the rotated/quantized domain.
	// Since the rotation is orthogonal, <R*x, R*y> = <x, y>.
	// The quantized dot product approximates the true dot product.
	indicesA := unpackCodes(a.Codes, a.Bits, a.Dim)
	indicesB := unpackCodes(b.Codes, b.Bits, b.Dim)

	dot := 0.0
	for i := 0; i < a.Dim; i++ {
		dot += cb.Dequantize(indicesA[i]) * cb.Dequantize(indicesB[i])
	}

	// Scale back: the quantized values are in the normalized+scaled domain,
	// so we need to undo the sqrt(d) scaling for both vectors and restore norms.
	d := float64(a.Dim)
	if d > 0 {
		dot = dot / d * a.Norm * b.Norm
	}

	return dot, nil
}

// CosineSimilarity estimates the cosine similarity between two quantized vectors.
func CosineSimilarity(a, b *QuantizedVector) (float64, error) {
	if err := checkCompatible(a, b); err != nil {
		return 0, err
	}
	if a.Norm == 0 || b.Norm == 0 {
		return 0, nil
	}

	ip, err := InnerProduct(a, b)
	if err != nil {
		return 0, err
	}

	return ip / (a.Norm * b.Norm), nil
}

// CompressionRatio returns the storage ratio: quantized size / original float64 size.
func CompressionRatio(dim, bits int) float64 {
	originalBytes := dim * 8                                    // float64 = 8 bytes
	quantizedBytes := packedSize(dim, bits) + 8 + 8 + 8 + 4 + 4 // codes + norm + seed + dim + bits overhead
	return float64(quantizedBytes) / float64(originalBytes)
}

// checkCompatible verifies two quantized vectors can be compared.
func checkCompatible(a, b *QuantizedVector) error {
	if a == nil || b == nil {
		return fmt.Errorf("cannot compare nil vectors")
	}
	if a.Dim != b.Dim {
		return fmt.Errorf("dimension mismatch: %d vs %d", a.Dim, b.Dim)
	}
	if a.Seed != b.Seed {
		return fmt.Errorf("seed mismatch: %d vs %d (vectors must use same rotation)", a.Seed, b.Seed)
	}
	if a.Bits != b.Bits {
		return fmt.Errorf("bit width mismatch: %d vs %d", a.Bits, b.Bits)
	}
	return nil
}

// packCodes packs uint8 indices into bytes. For b-bit codes, floor(8/b) codes
// fit per byte. Codes are packed MSB-first within each byte.
func packCodes(indices []uint8, bits int) []byte {
	codesPerByte := 8 / bits
	n := len(indices)
	packed := make([]byte, packedSize(n, bits))

	for i, idx := range indices {
		byteIdx := i / codesPerByte
		posInByte := i % codesPerByte
		shift := uint(8 - bits*(posInByte+1))
		mask := uint8((1 << bits) - 1)
		packed[byteIdx] |= (idx & mask) << shift
	}

	return packed
}

// unpackCodes extracts uint8 indices from packed bytes.
func unpackCodes(packed []byte, bits, dim int) []uint8 {
	codesPerByte := 8 / bits
	indices := make([]uint8, dim)
	mask := uint8((1 << bits) - 1)

	for i := 0; i < dim; i++ {
		byteIdx := i / codesPerByte
		posInByte := i % codesPerByte
		shift := uint(8 - bits*(posInByte+1))
		indices[i] = (packed[byteIdx] >> shift) & mask
	}

	return indices
}

// packedSize returns the number of bytes needed to store dim codes at the given bit width.
func packedSize(dim, bits int) int {
	codesPerByte := 8 / bits
	return (dim + codesPerByte - 1) / codesPerByte
}

// packZeroCodes creates the packed codes for a zero vector.
func packZeroCodes(dim int, cb *Codebook) []byte {
	// Find the index closest to 0
	zeroIdx := cb.Quantize(0)
	indices := make([]uint8, dim)
	for i := range indices {
		indices[i] = zeroIdx
	}
	return packCodes(indices, cb.Bits)
}
