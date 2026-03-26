package quantizer

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

func TestCodebookGetCodebook(t *testing.T) {
	for bits := 1; bits <= 4; bits++ {
		t.Run(fmt.Sprintf("bits=%d", bits), func(t *testing.T) {
			cb, err := GetCodebook(bits)
			if err != nil {
				t.Fatalf("GetCodebook(%d) failed: %v", bits, err)
			}
			if cb.Levels != 1<<bits {
				t.Errorf("expected %d levels, got %d", 1<<bits, cb.Levels)
			}
			if len(cb.Thresholds) != cb.Levels-1 {
				t.Errorf("expected %d thresholds, got %d", cb.Levels-1, len(cb.Thresholds))
			}
			if len(cb.Centers) != cb.Levels {
				t.Errorf("expected %d centers, got %d", cb.Levels, len(cb.Centers))
			}
			// Centers should be sorted
			for i := 1; i < len(cb.Centers); i++ {
				if cb.Centers[i] <= cb.Centers[i-1] {
					t.Errorf("centers not sorted: [%d]=%f >= [%d]=%f", i-1, cb.Centers[i-1], i, cb.Centers[i])
				}
			}
		})
	}

	t.Run("invalid_bits", func(t *testing.T) {
		_, err := GetCodebook(0)
		if err == nil {
			t.Error("expected error for bits=0")
		}
		_, err = GetCodebook(5)
		if err == nil {
			t.Error("expected error for bits=5")
		}
	})
}

func TestCodebookQuantizeDequantize(t *testing.T) {
	for bits := 1; bits <= 4; bits++ {
		t.Run(fmt.Sprintf("bits=%d", bits), func(t *testing.T) {
			cb, _ := GetCodebook(bits)
			// Quantize each center should map back to itself
			for i, center := range cb.Centers {
				idx := cb.Quantize(center)
				if int(idx) != i {
					t.Errorf("Quantize(%f) = %d, want %d", center, idx, i)
				}
				reconstructed := cb.Dequantize(idx)
				if reconstructed != center {
					t.Errorf("Dequantize(%d) = %f, want %f", idx, reconstructed, center)
				}
			}
		})
	}
}

func TestCodebookSymmetry(t *testing.T) {
	// Lloyd-Max codebooks for N(0,1) should be symmetric around 0
	for bits := 1; bits <= 4; bits++ {
		cb, _ := GetCodebook(bits)
		n := len(cb.Centers)
		for i := 0; i < n/2; i++ {
			if math.Abs(cb.Centers[i]+cb.Centers[n-1-i]) > 0.01 {
				t.Errorf("bits=%d: centers not symmetric: [%d]=%f, [%d]=%f",
					bits, i, cb.Centers[i], n-1-i, cb.Centers[n-1-i])
			}
		}
	}
}

func TestRotationDeterministic(t *testing.T) {
	dim := 64
	seed := int64(123)
	x1 := randomVector(dim, 1)
	x2 := make([]float64, dim)
	copy(x2, x1)

	r1 := NewRotation(dim, seed)
	r2 := NewRotation(dim, seed)

	r1.Apply(x1)
	r2.Apply(x2)

	for i := range x1 {
		if x1[i] != x2[i] {
			t.Fatalf("rotation not deterministic at index %d: %f vs %f", i, x1[i], x2[i])
		}
	}
}

func TestRotationPreservesNorm(t *testing.T) {
	dim := 128
	x := randomVector(dim, 42)
	normBefore := vectorNorm(x)

	rot := NewRotation(dim, 99)
	rot.Apply(x)
	normAfter := vectorNorm(x)

	relErr := math.Abs(normBefore-normAfter) / normBefore
	if relErr > 1e-10 {
		t.Errorf("rotation changed norm: before=%f, after=%f, relErr=%e", normBefore, normAfter, relErr)
	}
}

func TestRotationInverse(t *testing.T) {
	dim := 64
	original := randomVector(dim, 7)
	x := make([]float64, dim)
	copy(x, original)

	rot := NewRotation(dim, 55)
	rot.Apply(x)
	rot.ApplyInverse(x)

	for i := range x {
		if math.Abs(x[i]-original[i]) > 1e-10 {
			t.Fatalf("rotation inverse failed at index %d: got %f, want %f", i, x[i], original[i])
		}
	}
}

func TestQuantizeRoundTrip(t *testing.T) {
	dims := []int{8, 64, 384}
	bitWidths := []int{1, 2, 3, 4}

	for _, dim := range dims {
		for _, bits := range bitWidths {
			t.Run(fmt.Sprintf("dim=%d_bits=%d", dim, bits), func(t *testing.T) {
				vec := randomVector(dim, 42)
				cfg := Config{Bits: bits, Seed: 123}

				qv, err := Quantize(vec, cfg)
				if err != nil {
					t.Fatalf("Quantize failed: %v", err)
				}

				reconstructed, err := Dequantize(qv)
				if err != nil {
					t.Fatalf("Dequantize failed: %v", err)
				}

				if len(reconstructed) != dim {
					t.Fatalf("reconstructed dim %d, want %d", len(reconstructed), dim)
				}

				// Check direction is preserved (cosine similarity should be high for more bits)
				cosine := cosineSim(vec, reconstructed)
				minCosine := map[int]float64{1: 0.3, 2: 0.7, 3: 0.9, 4: 0.95}
				if cosine < minCosine[bits] {
					t.Errorf("cosine similarity %f below threshold %f", cosine, minCosine[bits])
				}
			})
		}
	}
}

func TestQuantizeZeroVector(t *testing.T) {
	vec := make([]float64, 64)
	cfg := DefaultConfig()

	qv, err := Quantize(vec, cfg)
	if err != nil {
		t.Fatalf("Quantize zero vector failed: %v", err)
	}
	if qv.Norm != 0 {
		t.Errorf("expected norm 0, got %f", qv.Norm)
	}
}

func TestQuantizeEmptyVector(t *testing.T) {
	_, err := Quantize([]float64{}, DefaultConfig())
	if err == nil {
		t.Error("expected error for empty vector")
	}
}

func TestQuantizeInvalidBits(t *testing.T) {
	vec := randomVector(8, 1)
	_, err := Quantize(vec, Config{Bits: 0, Seed: 1})
	if err == nil {
		t.Error("expected error for bits=0")
	}
	_, err = Quantize(vec, Config{Bits: 5, Seed: 1})
	if err == nil {
		t.Error("expected error for bits=5")
	}
}

func TestInnerProductAccuracy(t *testing.T) {
	dim := 384
	cfg := Config{Bits: 4, Seed: 42}

	// Test across many random vector pairs
	rng := rand.New(rand.NewSource(99))
	totalRelErr := 0.0
	trials := 100

	for trial := 0; trial < trials; trial++ {
		a := make([]float64, dim)
		b := make([]float64, dim)
		for i := range a {
			a[i] = rng.NormFloat64()
			b[i] = rng.NormFloat64()
		}

		exact := dotProduct(a, b)
		qa, _ := Quantize(a, cfg)
		qb, _ := Quantize(b, cfg)
		estimated, err := InnerProduct(qa, qb)
		if err != nil {
			t.Fatalf("InnerProduct failed: %v", err)
		}

		if exact != 0 {
			totalRelErr += math.Abs(estimated-exact) / math.Abs(exact)
		}
	}

	avgRelErr := totalRelErr / float64(trials)
	// At 4 bits, average relative error should be well under 50%
	if avgRelErr > 0.5 {
		t.Errorf("average relative inner product error too high: %f", avgRelErr)
	}
}

func TestCosineSimilarityIdentical(t *testing.T) {
	vec := randomVector(128, 42)
	cfg := Config{Bits: 4, Seed: 10}

	qa, _ := Quantize(vec, cfg)
	qb, _ := Quantize(vec, cfg)

	sim, err := CosineSimilarity(qa, qb)
	if err != nil {
		t.Fatalf("CosineSimilarity failed: %v", err)
	}
	if sim < 0.9 {
		t.Errorf("identical vectors should have cosine ~1.0, got %f", sim)
	}
}

func TestCosineSimilarityOrthogonal(t *testing.T) {
	// Construct two orthogonal vectors
	dim := 128
	a := make([]float64, dim)
	b := make([]float64, dim)
	for i := 0; i < dim/2; i++ {
		a[i] = 1.0
	}
	for i := dim / 2; i < dim; i++ {
		b[i] = 1.0
	}

	cfg := Config{Bits: 4, Seed: 10}
	qa, _ := Quantize(a, cfg)
	qb, _ := Quantize(b, cfg)

	sim, err := CosineSimilarity(qa, qb)
	if err != nil {
		t.Fatalf("CosineSimilarity failed: %v", err)
	}
	if math.Abs(sim) > 0.3 {
		t.Errorf("orthogonal vectors should have cosine ~0, got %f", sim)
	}
}

func TestDimensionMismatchError(t *testing.T) {
	cfg := Config{Bits: 2, Seed: 1}
	a, _ := Quantize(randomVector(64, 1), cfg)
	b, _ := Quantize(randomVector(128, 2), cfg)

	_, err := InnerProduct(a, b)
	if err == nil {
		t.Error("expected dimension mismatch error")
	}

	_, err = CosineSimilarity(a, b)
	if err == nil {
		t.Error("expected dimension mismatch error")
	}
}

func TestSeedMismatchError(t *testing.T) {
	a, _ := Quantize(randomVector(64, 1), Config{Bits: 2, Seed: 1})
	b, _ := Quantize(randomVector(64, 2), Config{Bits: 2, Seed: 2})

	_, err := InnerProduct(a, b)
	if err == nil {
		t.Error("expected seed mismatch error")
	}
}

func TestPackingRoundTrip(t *testing.T) {
	for bits := 1; bits <= 4; bits++ {
		t.Run(fmt.Sprintf("bits=%d", bits), func(t *testing.T) {
			maxVal := uint8((1 << bits) - 1)
			dim := 37 // non-aligned dimension to test edge cases
			indices := make([]uint8, dim)
			for i := range indices {
				indices[i] = uint8(i) % (maxVal + 1)
			}

			packed := packCodes(indices, bits)
			unpacked := unpackCodes(packed, bits, dim)

			for i := range indices {
				if unpacked[i] != indices[i] {
					t.Errorf("index %d: packed/unpacked mismatch: got %d, want %d", i, unpacked[i], indices[i])
				}
			}
		})
	}
}

func TestCompressionRatio(t *testing.T) {
	// 2-bit quantization of 384-dim vector
	ratio := CompressionRatio(384, 2)
	// 96 bytes packed + overhead vs 3072 bytes float64
	if ratio >= 0.1 {
		t.Errorf("compression ratio should be well under 10%%, got %f", ratio)
	}
}

// --- helpers ---

// suppress unused import lint
var _ = fmt.Sprintf

func randomVector(dim int, seed int64) []float64 {
	rng := rand.New(rand.NewSource(seed))
	v := make([]float64, dim)
	for i := range v {
		v[i] = rng.NormFloat64()
	}
	return v
}

func vectorNorm(v []float64) float64 {
	sum := 0.0
	for _, x := range v {
		sum += x * x
	}
	return math.Sqrt(sum)
}

func dotProduct(a, b []float64) float64 {
	sum := 0.0
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

func cosineSim(a, b []float64) float64 {
	dot := dotProduct(a, b)
	na := vectorNorm(a)
	nb := vectorNorm(b)
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (na * nb)
}
