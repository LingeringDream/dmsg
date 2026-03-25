package msg

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// Mine finds a nonce such that the first `difficulty` bits of
// SHA256(base + nonce) are zero.
// Difficulty is the number of leading zero bits (max 32 for practical use).
func Mine(base []byte, difficulty int) (uint64, error) {
	if difficulty < 0 || difficulty > 32 {
		return 0, fmt.Errorf("difficulty must be 0-32, got %d", difficulty)
	}
	if difficulty == 0 {
		return 0, nil
	}

	var buf [32]byte
	copy(buf[:], base)
	baseLen := len(base)
	if baseLen > 24 {
		// Ensure we have 8 bytes for nonce
		baseLen = 24
	}

	mask := uint32(0xFFFFFFFF) << (32 - difficulty)
	for nonce := uint64(0); nonce < 1_000_000_000; nonce++ {
		binary.BigEndian.PutUint64(buf[baseLen:], nonce)
		h := sha256.Sum256(buf[:baseLen+8])
		leading := binary.BigEndian.Uint32(h[:4])
		if leading&mask == 0 {
			return nonce, nil
		}
	}
	return 0, fmt.Errorf("PoW mining exceeded max iterations")
}

// CheckPoW verifies that hash(base + nonce) has `difficulty` leading zero bits.
func CheckPoW(base []byte, nonce uint64, difficulty int) bool {
	if difficulty <= 0 {
		return true
	}

	var buf [32]byte
	copy(buf[:], base)
	baseLen := len(base)
	if baseLen > 24 {
		baseLen = 24
	}
	binary.BigEndian.PutUint64(buf[baseLen:], nonce)
	h := sha256.Sum256(buf[:baseLen+8])

	mask := uint32(0xFFFFFFFF) << (32 - difficulty)
	return binary.BigEndian.Uint32(h[:4])&mask == 0
}

// DynamicDifficulty adjusts difficulty based on message rate.
// targetRate: messages per second the network should sustain.
// currentRate: actual messages per second observed locally.
// Returns recommended difficulty (leading zero bits).
func DynamicDifficulty(targetRate, currentRate float64) int {
	if currentRate <= 0 {
		return 8 // default
	}
	ratio := currentRate / targetRate
	// Each additional 4 bits ≈ 16x harder
	base := 8
	diff := base + int(ratio*4)
	if diff > 32 {
		diff = 32
	}
	if diff < 4 {
		diff = 4
	}
	return diff
}
