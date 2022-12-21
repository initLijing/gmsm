package cipher_test

import (
	"bytes"
	"crypto/cipher"
	"testing"

	"github.com/initLijing/gmsm/sm4"
)

type ofbTest struct {
	name string
	key  []byte
	iv   []byte
	in   []byte
	out  []byte
}

var ofbTests = []ofbTest{
	{
		"OFB-SM4",
		[]byte{0x2b, 0x7e, 0x15, 0x16, 0x28, 0xae, 0xd2, 0xa6, 0xab, 0xf7, 0x15, 0x88, 0x09, 0xcf, 0x4f, 0x3c},
		[]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f},
		[]byte{
			0x6b, 0xc1, 0xbe, 0xe2, 0x2e, 0x40, 0x9f, 0x96, 0xe9, 0x3d, 0x7e, 0x11, 0x73, 0x93, 0x17, 0x2a,
			0xae, 0x2d, 0x8a, 0x57, 0x1e, 0x03, 0xac, 0x9c, 0x9e, 0xb7, 0x6f, 0xac, 0x45, 0xaf, 0x8e, 0x51,
			0x30, 0xc8, 0x1c, 0x46, 0xa3, 0x5c, 0xe4, 0x11, 0xe5, 0xfb, 0xc1, 0x19, 0x1a, 0x0a, 0x52, 0xef,
			0xf6, 0x9f, 0x24, 0x45, 0xdf, 0x4f, 0x9b, 0x17, 0xad, 0x2b, 0x41, 0x7b, 0xe6, 0x6c, 0x37, 0x10,
		},
		[]byte{
			0xbc, 0x71, 0x0d, 0x76, 0x2d, 0x07, 0x0b, 0x26, 0x36, 0x1d, 0xa8, 0x2b, 0x54, 0x56, 0x5e, 0x46,
			0x07, 0xa0, 0xc6, 0x28, 0x34, 0x74, 0x0a, 0xd3, 0x24, 0x0d, 0x23, 0x91, 0x25, 0xe1, 0x16, 0x21,
			0xd4, 0x76, 0xb2, 0x1c, 0xc9, 0xf0, 0x49, 0x51, 0xf0, 0x74, 0x1d, 0x2e, 0xf9, 0xe0, 0x94, 0x98,
			0x15, 0x84, 0xfc, 0x14, 0x2b, 0xf1, 0x3a, 0xa6, 0x26, 0xb8, 0x2f, 0x9d, 0x7d, 0x07, 0x6c, 0xce,
		},
	},
}

func TestOFB(t *testing.T) {
	for _, tt := range ofbTests {
		test := tt.name

		c, err := sm4.NewCipher(tt.key)
		if err != nil {
			t.Errorf("%s: NewCipher(%d bytes) = %s", test, len(tt.key), err)
			continue
		}

		for j := 0; j <= 5; j += 5 {
			plaintext := tt.in[0 : len(tt.in)-j]
			ofb := cipher.NewOFB(c, tt.iv)
			ciphertext := make([]byte, len(plaintext))
			ofb.XORKeyStream(ciphertext, plaintext)
			if !bytes.Equal(ciphertext, tt.out[:len(plaintext)]) {
				t.Errorf("%s/%d: encrypting\ninput % x\nhave % x\nwant % x", test, len(plaintext), plaintext, ciphertext, tt.out)
			}
		}

		for j := 0; j <= 5; j += 5 {
			ciphertext := tt.out[0 : len(tt.in)-j]
			ofb := cipher.NewOFB(c, tt.iv)
			plaintext := make([]byte, len(ciphertext))
			ofb.XORKeyStream(plaintext, ciphertext)
			if !bytes.Equal(plaintext, tt.in[:len(ciphertext)]) {
				t.Errorf("%s/%d: decrypting\nhave % x\nwant % x", test, len(ciphertext), plaintext, tt.in)
			}
		}

		if t.Failed() {
			break
		}
	}
}
