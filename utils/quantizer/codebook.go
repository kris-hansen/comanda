package quantizer

import (
	"fmt"
	"math"
)

// Codebook holds precomputed Lloyd-Max quantization levels and thresholds
// for a given bit width, optimized for N(0,1) distributed inputs.
// These values are derived from the TurboQuant paper (arXiv 2504.19874),
// which shows that random rotation induces approximately normal coordinates.
type Codebook struct {
	Bits       int
	Levels     int       // 2^Bits
	Thresholds []float64 // len = Levels-1, decision boundaries
	Centers    []float64 // len = Levels, reconstruction values
}

// Precomputed Lloyd-Max optimal codebooks for N(0,1).
// These are the well-known optimal scalar quantizer values for the standard
// normal distribution, computed via the Lloyd-Max algorithm.
var codebooks = map[int]*Codebook{
	1: {
		Bits:       1,
		Levels:     2,
		Thresholds: []float64{0.0},
		Centers:    []float64{-0.7978845608, 0.7978845608}, // E[|X|] for half-normal
	},
	2: {
		Bits:       2,
		Levels:     4,
		Thresholds: []float64{-0.9816, 0.0, 0.9816},
		Centers:    []float64{-1.510, -0.4528, 0.4528, 1.510},
	},
	3: {
		Bits:       3,
		Levels:     8,
		Thresholds: []float64{-1.748, -1.050, -0.5006, 0.0, 0.5006, 1.050, 1.748},
		Centers:    []float64{-2.152, -1.344, -0.7560, -0.2451, 0.2451, 0.7560, 1.344, 2.152},
	},
	4: {
		Bits:       4,
		Levels:     16,
		Thresholds: []float64{-2.401, -1.844, -1.437, -1.099, -0.7975, -0.5224, -0.2582, 0.0, 0.2582, 0.5224, 0.7975, 1.099, 1.437, 1.844, 2.401},
		Centers:    []float64{-2.733, -2.069, -1.618, -1.256, -0.9424, -0.6568, -0.3881, -0.1284, 0.1284, 0.3881, 0.6568, 0.9424, 1.256, 1.618, 2.069, 2.733},
	},
}

// GetCodebook returns the precomputed Lloyd-Max codebook for a given bit width.
// Supported bit widths are 1, 2, 3, and 4.
func GetCodebook(bits int) (*Codebook, error) {
	cb, ok := codebooks[bits]
	if !ok {
		return nil, fmt.Errorf("unsupported bit width %d: must be 1, 2, 3, or 4", bits)
	}
	return cb, nil
}

// Quantize maps a scalar value to the nearest codebook index using binary search
// on the thresholds.
func (cb *Codebook) Quantize(x float64) uint8 {
	for i, t := range cb.Thresholds {
		if x < t {
			return uint8(i)
		}
	}
	return uint8(cb.Levels - 1)
}

// Dequantize maps a codebook index back to the reconstruction center value.
func (cb *Codebook) Dequantize(idx uint8) float64 {
	if int(idx) >= cb.Levels {
		return cb.Centers[cb.Levels-1]
	}
	return cb.Centers[idx]
}

// MSEDistortion returns the theoretical mean squared error for this codebook
// when applied to N(0,1) inputs, following the TurboQuant bound:
// D_mse <= (sqrt(3)*pi/2) * (1/4^b)
func (cb *Codebook) MSEDistortion() float64 {
	return (math.Sqrt(3) * math.Pi / 2.0) * (1.0 / math.Pow(4.0, float64(cb.Bits)))
}
