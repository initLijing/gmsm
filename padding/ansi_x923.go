package padding

import (
	"errors"

	"github.com/initLijing/gmsm/internal/alias"
)

// https://www.ibm.com/docs/en/linux-on-systems?topic=processes-ansi-x923-cipher-block-chaining
type ansiX923Padding uint

func (pad ansiX923Padding) BlockSize() int {
	return int(pad)
}

func (pad ansiX923Padding) Pad(src []byte) []byte {
	overhead := pad.BlockSize() - len(src)%pad.BlockSize()
	ret, out := alias.SliceForAppend(src, overhead)
	out[overhead-1] = byte(overhead)
	for i := 0; i < overhead-1; i++ {
		out[i] = 0
	}
	return ret
}

// Unpad decrypted plaintext, non-constant-time
func (pad ansiX923Padding) Unpad(src []byte) ([]byte, error) {
	srcLen := len(src)
	if srcLen == 0 || srcLen%pad.BlockSize() != 0 {
		return nil, errors.New("ansi x9.23: src length is not multiple of block size")
	}
	paddedLen := src[srcLen-1]
	if paddedLen == 0 || int(paddedLen) > pad.BlockSize() {
		return nil, errors.New("ansi x9.23: invalid padding length")
	}
	for _, b := range src[srcLen-int(paddedLen) : srcLen-1] {
		if b != 0 {
			return nil, errors.New("ansi x9.23: invalid padding bytes")
		}
	}
	return src[:srcLen-int(paddedLen)], nil
}
