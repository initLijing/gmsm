package cipher_test

import (
	"bytes"
	"crypto/cipher"
	"testing"

	"github.com/initLijing/gmsm/sm4"
)

var commonCounter = []byte{0xf0, 0xf1, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6, 0xf7, 0xf8, 0xf9, 0xfa, 0xfb, 0xfc, 0xfd, 0xfe, 0xff}

var ctrSM4Tests = []struct {
	name string
	key  []byte
	iv   []byte
	in   []byte
	out  []byte
}{
	{
		"2 blocks",
		[]byte{0x2b, 0x7e, 0x15, 0x16, 0x28, 0xae, 0xd2, 0xa6, 0xab, 0xf7, 0x15, 0x88, 0x09, 0xcf, 0x4f, 0x3c},
		[]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f},
		[]byte{
			0x6b, 0xc1, 0xbe, 0xe2, 0x2e, 0x40, 0x9f, 0x96, 0xe9, 0x3d, 0x7e, 0x11, 0x73, 0x93, 0x17, 0x2a,
			0xae, 0x2d, 0x8a, 0x57, 0x1e, 0x03, 0xac, 0x9c, 0x9e, 0xb7, 0x6f, 0xac, 0x45, 0xaf, 0x8e, 0x51},
		[]byte{
			0xbc, 0x71, 0x0d, 0x76, 0x2d, 0x07, 0x0b, 0x26, 0x36, 0x1d, 0xa8, 0x2b, 0x54, 0x56, 0x5e, 0x46,
			0xb0, 0x2b, 0x3d, 0xbd, 0xdd, 0x50, 0xd5, 0xb4, 0x58, 0xae, 0xcc, 0xb2, 0x5d, 0xa1, 0x05, 0xe1},
	},
}

func TestCTR_SM4(t *testing.T) {
	for _, tt := range ctrSM4Tests {
		test := tt.name

		c, err := sm4.NewCipher(tt.key)
		if err != nil {
			t.Errorf("%s: NewCipher(%d bytes) = %s", test, len(tt.key), err)
			continue
		}

		for j := 0; j <= 5; j += 5 {
			in := tt.in[0 : len(tt.in)-j]
			ctr := cipher.NewCTR(c, tt.iv)
			encrypted := make([]byte, len(in))
			ctr.XORKeyStream(encrypted, in)
			if out := tt.out[0:len(in)]; !bytes.Equal(out, encrypted) {
				t.Errorf("%s/%d: CTR\ninpt %x\nhave %x\nwant %x", test, len(in), in, encrypted, out)
			}
		}

		for j := 0; j <= 7; j += 7 {
			in := tt.out[0 : len(tt.out)-j]
			ctr := cipher.NewCTR(c, tt.iv)
			plain := make([]byte, len(in))
			ctr.XORKeyStream(plain, in)
			if out := tt.in[0:len(in)]; !bytes.Equal(out, plain) {
				t.Errorf("%s/%d: CTRReader\nhave %x\nwant %x", test, len(out), plain, out)
			}
		}

		if t.Failed() {
			break
		}
	}
}
