package nir

import (
	"fmt"
	"math"
	"strconv"
)

// EncodeNumber preserves signed zero, infinities and every NaN payload.
func EncodeNumber(value float64) string {
	return fmt.Sprintf("%016x", math.Float64bits(value))
}

func DecodeNumber(bits string) (float64, error) {
	if len(bits) != 16 {
		return 0, fmt.Errorf("binary64 requires 16 hexadecimal digits, got %q", bits)
	}
	for _, ch := range bits {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return 0, fmt.Errorf("noncanonical binary64 bits %q", bits)
		}
	}
	u, err := strconv.ParseUint(bits, 16, 64)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(u), nil
}
