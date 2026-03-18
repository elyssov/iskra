package identity

import (
	"fmt"
	"math/big"
)

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// ToBase58 encodes bytes to base58 string.
func ToBase58(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	// Count leading zeros
	leadingZeros := 0
	for _, b := range data {
		if b != 0 {
			break
		}
		leadingZeros++
	}

	x := new(big.Int).SetBytes(data)
	base := big.NewInt(58)
	zero := big.NewInt(0)
	mod := new(big.Int)

	var result []byte
	for x.Cmp(zero) > 0 {
		x.DivMod(x, base, mod)
		result = append(result, base58Alphabet[mod.Int64()])
	}

	// Add leading '1's for each leading zero byte
	for i := 0; i < leadingZeros; i++ {
		result = append(result, base58Alphabet[0])
	}

	// Reverse
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return string(result)
}

// FromBase58 decodes a base58 string back to bytes.
func FromBase58(s string) ([]byte, error) {
	if len(s) == 0 {
		return []byte{}, nil
	}

	x := big.NewInt(0)
	base := big.NewInt(58)

	for _, c := range []byte(s) {
		idx := -1
		for i := 0; i < len(base58Alphabet); i++ {
			if base58Alphabet[i] == c {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, fmt.Errorf("invalid base58 character: %c", c)
		}
		x.Mul(x, base)
		x.Add(x, big.NewInt(int64(idx)))
	}

	// Count leading '1's (zero bytes)
	leadingOnes := 0
	for _, c := range []byte(s) {
		if c != base58Alphabet[0] {
			break
		}
		leadingOnes++
	}

	decoded := x.Bytes()
	result := make([]byte, leadingOnes+len(decoded))
	copy(result[leadingOnes:], decoded)

	return result, nil
}
