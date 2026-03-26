package quantizer

import (
	"math"
	"math/rand"
)

// RotationMatrix represents a random orthogonal rotation implemented via
// chained Householder reflections. This avoids materializing the full d×d
// matrix, keeping memory at O(k*d) where k is the number of reflections.
//
// The rotation is data-oblivious (depends only on the seed), which is a
// key property from TurboQuant: the same rotation works for any input
// distribution, and after rotation each coordinate is approximately N(0, 1/d)
// for unit-norm vectors.
type RotationMatrix struct {
	dim        int
	seed       int64
	reflectors [][]float64 // k Householder vectors
}

// numReflections determines how many Householder reflections to use.
// Empirically, 3*ceil(log2(d)) provides good mixing.
func numReflections(dim int) int {
	if dim <= 1 {
		return 0
	}
	k := int(math.Ceil(math.Log2(float64(dim)))) * 3
	if k < 3 {
		k = 3
	}
	return k
}

// NewRotation creates a random orthogonal rotation for the given dimension
// using the specified seed. The rotation is deterministic for the same
// (dim, seed) pair.
func NewRotation(dim int, seed int64) *RotationMatrix {
	r := &RotationMatrix{
		dim:  dim,
		seed: seed,
	}
	rng := rand.New(rand.NewSource(seed)) //nolint:gosec // deterministic seeded RNG is intentional for reproducible rotations
	k := numReflections(dim)
	r.reflectors = make([][]float64, k)

	for i := 0; i < k; i++ {
		// Generate a random unit vector for the Householder reflection
		v := make([]float64, dim)
		norm := 0.0
		for j := 0; j < dim; j++ {
			v[j] = rng.NormFloat64()
			norm += v[j] * v[j]
		}
		norm = math.Sqrt(norm)
		if norm > 0 {
			for j := 0; j < dim; j++ {
				v[j] /= norm
			}
		}
		r.reflectors[i] = v
	}

	return r
}

// Apply rotates vector x in-place using the chain of Householder reflections.
// Each reflection is: x ← x - 2*(v·x)*v
func (r *RotationMatrix) Apply(x []float64) {
	for _, v := range r.reflectors {
		applyHouseholder(x, v)
	}
}

// ApplyInverse rotates vector x in-place by the inverse rotation (R^T).
// Since each Householder reflection is its own inverse, we apply them
// in reverse order.
func (r *RotationMatrix) ApplyInverse(x []float64) {
	for i := len(r.reflectors) - 1; i >= 0; i-- {
		applyHouseholder(x, r.reflectors[i])
	}
}

// applyHouseholder applies a single Householder reflection in-place:
// x ← x - 2*(v·x)*v
func applyHouseholder(x, v []float64) {
	dot := 0.0
	for i := range v {
		dot += v[i] * x[i]
	}
	scale := 2.0 * dot
	for i := range v {
		x[i] -= scale * v[i]
	}
}
