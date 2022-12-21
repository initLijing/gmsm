// Package sm9 handle shangmi sm9 algorithm and its curves and pairing implementation
package sm9

import (
	"crypto"
	goSubtle "crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/initLijing/gmsm/internal/bigmod"
	"github.com/initLijing/gmsm/internal/subtle"
	"github.com/initLijing/gmsm/kdf"
	"github.com/initLijing/gmsm/sm3"
	"github.com/initLijing/gmsm/sm9/bn256"
	"golang.org/x/crypto/cryptobyte"
	"golang.org/x/crypto/cryptobyte/asn1"
)

// SM9 ASN.1 format reference: Information security technology - SM9 cryptographic algorithm application specification

var orderNat = bigmod.NewModulusFromBig(bn256.Order)
var orderMinus2 = new(big.Int).Sub(bn256.Order, big.NewInt(2)).Bytes()
var bigOne = big.NewInt(1)
var bigOneNat *bigmod.Nat
var orderMinus1 = bigmod.NewNat().SetBig(new(big.Int).Sub(bn256.Order, bigOne))

func init() {
	bigOneNat, _ = bigmod.NewNat().SetBytes(bigOne.Bytes(), orderNat)
}

type hashMode byte

const (
	// hashmode used in h1: 0x01
	H1 hashMode = 1 + iota
	// hashmode used in h2: 0x02
	H2
)

type encryptType byte

const (
	ENC_TYPE_XOR encryptType = 0
	ENC_TYPE_ECB encryptType = 1
	ENC_TYPE_CBC encryptType = 2
	ENC_TYPE_OFB encryptType = 4
	ENC_TYPE_CFB encryptType = 8
)

//hash implements H1(Z,n) or H2(Z,n) in sm9 algorithm.
func hash(z []byte, h hashMode) *bigmod.Nat {
	md := sm3.New()
	var ha [64]byte
	var countBytes [4]byte
	var ct uint32 = 1

	for i := 0; i < 2; i++ {
		binary.BigEndian.PutUint32(countBytes[:], ct)
		md.Write([]byte{byte(h)})
		md.Write(z)
		md.Write(countBytes[:])
		copy(ha[i*sm3.Size:], md.Sum(nil))
		ct++
		md.Reset()
	}
	k := new(big.Int).SetBytes(ha[:40])
	kNat := bigmod.NewNat().SetBig(k)
	kNat = bigmod.NewNat().ModNat(kNat, orderMinus1)
	kNat.Add(bigOneNat, orderNat)
	return kNat
}

func hashH1(z []byte) *bigmod.Nat {
	return hash(z, H1)
}

func hashH2(z []byte) *bigmod.Nat {
	return hash(z, H2)
}

func randomScalar(rand io.Reader) (k *bigmod.Nat, err error) {
	k = bigmod.NewNat()
	for {
		b := make([]byte, orderNat.Size())
		if _, err = io.ReadFull(rand, b); err != nil {
			return
		}

		// Mask off any excess bits to increase the chance of hitting a value in
		// (0, N). These are the most dangerous lines in the package and maybe in
		// the library: a single bit of bias in the selection of nonces would likely
		// lead to key recovery, but no tests would fail. Look but DO NOT TOUCH.
		if excess := len(b)*8 - orderNat.BitLen(); excess > 0 {
			// Just to be safe, assert that this only happens for the one curve that
			// doesn't have a round number of bits.
			if excess != 0 {
				panic("sm9: internal error: unexpectedly masking off bits")
			}
			b[0] >>= excess
		}

		// FIPS 186-4 makes us check k <= N - 2 and then add one.
		// Checking 0 < k <= N - 1 is strictly equivalent.
		// None of this matters anyway because the chance of selecting
		// zero is cryptographically negligible.
		if _, err = k.SetBytes(b, orderNat); err == nil && k.IsZero() == 0 {
			break
		}
	}
	return
}

// Sign signs a hash (which should be the result of hashing a larger message)
// using the user dsa key. It returns the signature as a pair of h and s.
// Please use SignASN1 instead.
func Sign(rand io.Reader, priv *SignPrivateKey, hash []byte) (h *big.Int, s *bn256.G1, err error) {
	sig, err := SignASN1(rand, priv, hash)
	if err != nil {
		return nil, nil, err
	}
	return parseSignatureLegacy(sig)
}

// Sign signs digest with user's DSA key, reading randomness from rand. The opts argument
// is not currently used but, in keeping with the crypto.Signer interface.
// The result is SM9Signature ASN.1 format.
func (priv *SignPrivateKey) Sign(rand io.Reader, hash []byte, opts crypto.SignerOpts) ([]byte, error) {
	return SignASN1(rand, priv, hash)
}

// SignASN1 signs a hash (which should be the result of hashing a larger message)
// using the private key, priv. It returns the ASN.1 encoded signature of type SM9Signature.
func SignASN1(rand io.Reader, priv *SignPrivateKey, hash []byte) ([]byte, error) {
	var (
		hNat *bigmod.Nat
		s    *bn256.G1
	)
	for {
		r, err := randomScalar(rand)
		if err != nil {
			return nil, err
		}

		w, err := priv.SignMasterPublicKey.ScalarBaseMult(r.Bytes(orderNat))
		if err != nil {
			return nil, err
		}

		var buffer []byte
		buffer = append(buffer, hash...)
		buffer = append(buffer, w.Marshal()...)

		hNat = hashH2(buffer)
		r.Sub(hNat, orderNat)

		if r.IsZero() == 0 {
			s, err = new(bn256.G1).ScalarMult(priv.PrivateKey, r.Bytes(orderNat))
			if err != nil {
				return nil, err
			}
			break
		}
	}

	return encodeSignature(hNat.Bytes(orderNat), s)
}

// Verify verifies the signature in h, s of hash using the master dsa public key and user id, uid and hid.
// Its return value records whether the signature is valid. Please use VerifyASN1 instead.
func Verify(pub *SignMasterPublicKey, uid []byte, hid byte, hash []byte, h *big.Int, s *bn256.G1) bool {
	if h.Sign() <= 0 {
		return false
	}
	sig, err := encodeSignature(h.Bytes(), s)
	if err != nil {
		return false
	}
	return VerifyASN1(pub, uid, hid, hash, sig)
}

func encodeSignature(hBytes []byte, s *bn256.G1) ([]byte, error) {
	var b cryptobyte.Builder
	b.AddASN1(asn1.SEQUENCE, func(b *cryptobyte.Builder) {
		b.AddASN1OctetString(hBytes)
		b.AddASN1BitString(s.MarshalUncompressed())
	})
	return b.Bytes()
}

func parseSignature(sig []byte) ([]byte, *bn256.G1, error) {
	var (
		hBytes []byte
		sBytes []byte
		inner  cryptobyte.String
	)
	input := cryptobyte.String(sig)
	if !input.ReadASN1(&inner, asn1.SEQUENCE) ||
		!input.Empty() ||
		!inner.ReadASN1Bytes(&hBytes, asn1.OCTET_STRING) ||
		!inner.ReadASN1BitStringAsBytes(&sBytes) ||
		!inner.Empty() {
		return nil, nil, errors.New("invalid ASN.1")
	}
	if sBytes[0] != 4 {
		return nil, nil, errors.New("sm9: invalid point format")
	}
	s := new(bn256.G1)
	_, err := s.Unmarshal(sBytes[1:])
	if err != nil {
		return nil, nil, err
	}
	return hBytes, s, nil
}

func parseSignatureLegacy(sig []byte) (*big.Int, *bn256.G1, error) {
	hBytes, s, err := parseSignature(sig)
	if err != nil {
		return nil, nil, err
	}
	return new(big.Int).SetBytes(hBytes), s, nil
}

// VerifyASN1 verifies the ASN.1 encoded signature of type SM9Signature, sig, of hash using the
// public key, pub. Its return value records whether the signature is valid.
func VerifyASN1(pub *SignMasterPublicKey, uid []byte, hid byte, hash, sig []byte) bool {
	h, s, err := parseSignature(sig)
	if err != nil {
		return false
	}
	if !s.IsOnCurve() {
		return false
	}

	hNat, err := bigmod.NewNat().SetBytes(h, orderNat)
	if err != nil {
		return false
	}
	if hNat.IsZero() == 1 {
		return false
	}

	t, err := pub.ScalarBaseMult(hNat.Bytes(orderNat))
	if err != nil {
		return false
	}

	// user sign public key p generation
	p := pub.GenerateUserPublicKey(uid, hid)

	u := bn256.Pair(s, p)
	w := new(bn256.GT).Add(u, t)

	var buffer []byte
	buffer = append(buffer, hash...)
	buffer = append(buffer, w.Marshal()...)
	h2 := hashH2(buffer)

	return h2.Equal(hNat) == 1
}

// Verify verifies the ASN.1 encoded signature, sig, of hash using the
// public key, pub. Its return value records whether the signature is valid.
func (pub *SignMasterPublicKey) Verify(uid []byte, hid byte, hash, sig []byte) bool {
	return VerifyASN1(pub, uid, hid, hash, sig)
}

// WrapKey generate and wrap key with reciever's uid and system hid
func WrapKey(rand io.Reader, pub *EncryptMasterPublicKey, uid []byte, hid byte, kLen int) (key []byte, cipher *bn256.G1, err error) {
	q := pub.GenerateUserPublicKey(uid, hid)
	var (
		r *bigmod.Nat
		w *bn256.GT
	)
	for {
		r, err = randomScalar(rand)
		if err != nil {
			return
		}

		rBytes := r.Bytes(orderNat)
		cipher, err = new(bn256.G1).ScalarMult(q, rBytes)
		if err != nil {
			return
		}

		w, err = pub.ScalarBaseMult(rBytes)
		if err != nil {
			return
		}
		var buffer []byte
		buffer = append(buffer, cipher.Marshal()...)
		buffer = append(buffer, w.Marshal()...)
		buffer = append(buffer, uid...)

		key = kdf.Kdf(sm3.New(), buffer, kLen)
		if !subtle.ConstantTimeAllZero(key) {
			break
		}
	}
	return
}

// WrapKey wrap key and marshal the cipher as ASN1 format, SM9PublicKey1 definition.
func (pub *EncryptMasterPublicKey) WrapKey(rand io.Reader, uid []byte, hid byte, kLen int) ([]byte, []byte, error) {
	key, cipher, err := WrapKey(rand, pub, uid, hid, kLen)
	if err != nil {
		return nil, nil, err
	}
	var b cryptobyte.Builder
	b.AddASN1BitString(cipher.MarshalUncompressed())
	cipherASN1, err := b.Bytes()

	return key, cipherASN1, err
}

// WrapKeyASN1 wrap key and marshal the result of SM9KeyPackage as ASN1 format. according
// SM9 cryptographic algorithm application specification, SM9KeyPackage defnition.
func (pub *EncryptMasterPublicKey) WrapKeyASN1(rand io.Reader, uid []byte, hid byte, kLen int) ([]byte, error) {
	key, cipher, err := WrapKey(rand, pub, uid, hid, kLen)
	if err != nil {
		return nil, err
	}
	var b cryptobyte.Builder
	b.AddASN1(asn1.SEQUENCE, func(b *cryptobyte.Builder) {
		b.AddASN1OctetString(key)
		b.AddASN1BitString(cipher.MarshalUncompressed())
	})
	return b.Bytes()
}

// UnmarshalSM9KeyPackage is an utility to unmarshal SM9KeyPackage
func UnmarshalSM9KeyPackage(der []byte) ([]byte, *bn256.G1, error) {
	input := cryptobyte.String(der)
	var (
		key         []byte
		cipherBytes []byte
		inner       cryptobyte.String
	)
	if !input.ReadASN1(&inner, asn1.SEQUENCE) ||
		!input.Empty() ||
		!inner.ReadASN1Bytes(&key, asn1.OCTET_STRING) ||
		!inner.ReadASN1BitStringAsBytes(&cipherBytes) ||
		!inner.Empty() {
		return nil, nil, errors.New("sm9: invalid SM9KeyPackage asn.1 data")
	}
	g, err := unmarshalG1(cipherBytes)
	if err != nil {
		return nil, nil, err
	}
	return key, g, nil
}

// ErrDecryption represents a failure to decrypt a message.
// It is deliberately vague to avoid adaptive attacks.
var ErrDecryption = errors.New("sm9: decryption error")

// UnwrapKey unwrap key from cipher, user id and aligned key length
func UnwrapKey(priv *EncryptPrivateKey, uid []byte, cipher *bn256.G1, kLen int) ([]byte, error) {
	if !cipher.IsOnCurve() {
		return nil, ErrDecryption
	}

	w := bn256.Pair(cipher, priv.PrivateKey)

	var buffer []byte
	buffer = append(buffer, cipher.Marshal()...)
	buffer = append(buffer, w.Marshal()...)
	buffer = append(buffer, uid...)

	key := kdf.Kdf(sm3.New(), buffer, kLen)
	if subtle.ConstantTimeAllZero(key) {
		return nil, ErrDecryption
	}
	return key, nil
}

// UnwrapKey unwrap key from cipherDer, user id and aligned key length.
// cipherDer is SM9PublicKey1 format according SM9 cryptographic algorithm application specification.
func (priv *EncryptPrivateKey) UnwrapKey(uid, cipherDer []byte, kLen int) ([]byte, error) {
	var bytes []byte
	input := cryptobyte.String(cipherDer)
	if !input.ReadASN1BitStringAsBytes(&bytes) || !input.Empty() {
		return nil, ErrDecryption
	}
	g, err := unmarshalG1(bytes)
	if err != nil {
		return nil, ErrDecryption
	}
	return UnwrapKey(priv, uid, g, kLen)
}

// Encrypt encrypt plaintext, output ciphertext with format C1||C3||C2
func Encrypt(rand io.Reader, pub *EncryptMasterPublicKey, uid []byte, hid byte, plaintext []byte) ([]byte, error) {
	key, cipher, err := WrapKey(rand, pub, uid, hid, len(plaintext)+sm3.Size)
	if err != nil {
		return nil, err
	}
	subtle.XORBytes(key, key[:len(plaintext)], plaintext)

	hash := sm3.New()
	hash.Write(key)
	c3 := hash.Sum(nil)

	ciphertext := append(cipher.Marshal(), c3...)
	ciphertext = append(ciphertext, key[:len(plaintext)]...)
	return ciphertext, nil
}

// EncryptASN1 encrypt plaintext and output ciphertext with ASN.1 format according
// SM9 cryptographic algorithm application specification, SM9Cipher definition.
func EncryptASN1(rand io.Reader, pub *EncryptMasterPublicKey, uid []byte, hid byte, plaintext []byte) ([]byte, error) {
	return pub.Encrypt(rand, uid, hid, plaintext)
}

// Encrypt encrypt plaintext and output ciphertext with ASN.1 format according
// SM9 cryptographic algorithm application specification, SM9Cipher definition.
func (pub *EncryptMasterPublicKey) Encrypt(rand io.Reader, uid []byte, hid byte, plaintext []byte) ([]byte, error) {
	key, cipher, err := WrapKey(rand, pub, uid, hid, len(plaintext)+sm3.Size)
	if err != nil {
		return nil, err
	}
	subtle.XORBytes(key, key[:len(plaintext)], plaintext)

	hash := sm3.New()
	hash.Write(key)
	c3 := hash.Sum(nil)

	var b cryptobyte.Builder
	b.AddASN1(asn1.SEQUENCE, func(b *cryptobyte.Builder) {
		b.AddASN1Int64(int64(ENC_TYPE_XOR))
		b.AddASN1BitString(cipher.MarshalUncompressed())
		b.AddASN1OctetString(c3)
		b.AddASN1OctetString(key[:len(plaintext)])
	})
	return b.Bytes()
}

// Decrypt decrypt chipher, ciphertext should be with format C1||C3||C2
func Decrypt(priv *EncryptPrivateKey, uid, ciphertext []byte) ([]byte, error) {
	c := &bn256.G1{}
	c3, err := c.Unmarshal(ciphertext)
	if err != nil {
		return nil, ErrDecryption
	}

	key, err := UnwrapKey(priv, uid, c, len(c3))
	if err != nil {
		return nil, err
	}

	c2 := c3[sm3.Size:]

	hash := sm3.New()
	hash.Write(c2)
	hash.Write(key[len(c2):])
	c32 := hash.Sum(nil)

	if goSubtle.ConstantTimeCompare(c3[:sm3.Size], c32) != 1 {
		return nil, ErrDecryption
	}

	subtle.XORBytes(key, c2, key[:len(c2)])
	return key[:len(c2)], nil
}

// DecryptASN1 decrypt chipher, ciphertext should be with ASN.1 format according
// SM9 cryptographic algorithm application specification, SM9Cipher definition.
func DecryptASN1(priv *EncryptPrivateKey, uid, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) <= 32+65 {
		return nil, errors.New("sm9: ciphertext too short")
	}
	var (
		encType int
		c3Bytes []byte
		c1Bytes []byte
		c2Bytes []byte
		inner   cryptobyte.String
	)
	input := cryptobyte.String(ciphertext)
	if !input.ReadASN1(&inner, asn1.SEQUENCE) ||
		!input.Empty() ||
		!inner.ReadASN1Integer(&encType) ||
		!inner.ReadASN1BitStringAsBytes(&c1Bytes) ||
		!inner.ReadASN1Bytes(&c3Bytes, asn1.OCTET_STRING) ||
		!inner.ReadASN1Bytes(&c2Bytes, asn1.OCTET_STRING) ||
		!inner.Empty() {
		return nil, errors.New("sm9: invalid ciphertext asn.1 data")
	}
	if encType != int(ENC_TYPE_XOR) {
		return nil, fmt.Errorf("sm9: does not support this kind of encrypt type <%v> yet", encType)
	}
	c, err := unmarshalG1(c1Bytes)
	if err != nil {
		return nil, ErrDecryption
	}

	key, err := UnwrapKey(priv, uid, c, len(c2Bytes)+len(c3Bytes))
	if err != nil {
		return nil, err
	}

	hash := sm3.New()
	hash.Write(c2Bytes)
	hash.Write(key[len(c2Bytes):])
	c32 := hash.Sum(nil)

	if goSubtle.ConstantTimeCompare(c3Bytes, c32) != 1 {
		return nil, ErrDecryption
	}
	subtle.XORBytes(key, c2Bytes, key[:len(c2Bytes)])
	return key[:len(c2Bytes)], nil
}

// Decrypt decrypt chipher, ciphertext should be with ASN.1 format according
// SM9 cryptographic algorithm application specification, SM9Cipher definition.
func (priv *EncryptPrivateKey) Decrypt(uid, ciphertext []byte) ([]byte, error) {
	if ciphertext[0] == 0x30 { // should be ASN.1 format
		return DecryptASN1(priv, uid, ciphertext)
	}
	// fallback to C1||C3||C2 raw format
	return Decrypt(priv, uid, ciphertext)
}

// KeyExchange key exchange struct, include internal stat in whole key exchange flow.
// Initiator's flow will be: NewKeyExchange -> InitKeyExchange -> transmission -> ConfirmResponder
// Responder's flow will be: NewKeyExchange -> waiting ... -> RepondKeyExchange -> transmission -> ConfirmInitiator
type KeyExchange struct {
	genSignature bool               // control the optional sign/verify step triggered by responsder
	keyLength    int                // key length
	privateKey   *EncryptPrivateKey // owner's encryption private key
	uid          []byte             // owner uid
	peerUID      []byte             // peer uid
	r            *bigmod.Nat        // random which will be used to compute secret
	secret       *bn256.G1          // generated secret which will be passed to peer
	peerSecret   *bn256.G1          // received peer's secret
	g1           *bn256.GT          // internal state which will be used when compute the key and signature
	g2           *bn256.GT          // internal state which will be used when compute the key and signature
	g3           *bn256.GT          // internal state which will be used when compute the key and signature
}

// NewKeyExchange create one new KeyExchange object
func NewKeyExchange(priv *EncryptPrivateKey, uid, peerUID []byte, keyLen int, genSignature bool) *KeyExchange {
	ke := &KeyExchange{}
	ke.genSignature = genSignature
	ke.keyLength = keyLen
	ke.privateKey = priv
	ke.uid = uid
	ke.peerUID = peerUID
	return ke
}

// Destroy clear all internal state and Ephemeral private/public keys
func (ke *KeyExchange) Destroy() {
	if ke.r != nil {
		ke.r.SetBytes([]byte{0}, orderNat)
	}
	if ke.g1 != nil {
		ke.g1.SetOne()
	}
	if ke.g2 != nil {
		ke.g2.SetOne()
	}
	if ke.g3 != nil {
		ke.g3.SetOne()
	}
}

func initKeyExchange(ke *KeyExchange, hid byte, r *bigmod.Nat) {
	pubB := ke.privateKey.GenerateUserPublicKey(ke.peerUID, hid)
	ke.r = r
	rA, err := new(bn256.G1).ScalarMult(pubB, ke.r.Bytes(orderNat))
	if err != nil {
		panic(err)
	}
	ke.secret = rA
}

// InitKeyExchange generate random with responder uid, for initiator's step A1-A4
func (ke *KeyExchange) InitKeyExchange(rand io.Reader, hid byte) (*bn256.G1, error) {
	r, err := randomScalar(rand)
	if err != nil {
		return nil, err
	}
	initKeyExchange(ke, hid, r)
	return ke.secret, nil
}

func (ke *KeyExchange) sign(isResponder bool, prefix byte) []byte {
	var buffer []byte
	hash := sm3.New()
	hash.Write(ke.g2.Marshal())
	hash.Write(ke.g3.Marshal())
	if isResponder {
		hash.Write(ke.peerUID)
		hash.Write(ke.uid)
		hash.Write(ke.peerSecret.Marshal())
		hash.Write(ke.secret.Marshal())
	} else {
		hash.Write(ke.uid)
		hash.Write(ke.peerUID)
		hash.Write(ke.secret.Marshal())
		hash.Write(ke.peerSecret.Marshal())
	}
	buffer = hash.Sum(nil)
	hash.Reset()
	hash.Write([]byte{prefix})
	hash.Write(ke.g1.Marshal())
	hash.Write(buffer)
	return hash.Sum(nil)
}

func (ke *KeyExchange) generateSharedKey(isResponder bool) ([]byte, error) {
	var buffer []byte
	if isResponder {
		buffer = append(buffer, ke.peerUID...)
		buffer = append(buffer, ke.uid...)
		buffer = append(buffer, ke.peerSecret.Marshal()...)
		buffer = append(buffer, ke.secret.Marshal()...)
	} else {
		buffer = append(buffer, ke.uid...)
		buffer = append(buffer, ke.peerUID...)
		buffer = append(buffer, ke.secret.Marshal()...)
		buffer = append(buffer, ke.peerSecret.Marshal()...)
	}
	buffer = append(buffer, ke.g1.Marshal()...)
	buffer = append(buffer, ke.g2.Marshal()...)
	buffer = append(buffer, ke.g3.Marshal()...)

	return kdf.Kdf(sm3.New(), buffer, ke.keyLength), nil
}

func respondKeyExchange(ke *KeyExchange, hid byte, r *bigmod.Nat, rA *bn256.G1) (*bn256.G1, []byte, error) {
	if !rA.IsOnCurve() {
		return nil, nil, errors.New("sm9: invalid initiator's ephemeral public key")
	}
	ke.peerSecret = rA
	pubA := ke.privateKey.GenerateUserPublicKey(ke.peerUID, hid)
	ke.r = r
	rBytes := r.Bytes(orderNat)
	rB, err := new(bn256.G1).ScalarMult(pubA, rBytes)
	if err != nil {
		return nil, nil, err
	}
	ke.secret = rB

	ke.g1 = bn256.Pair(ke.peerSecret, ke.privateKey.PrivateKey)
	ke.g3 = &bn256.GT{}
	g3, err := bn256.ScalarMultGT(ke.g1, rBytes)
	if err != nil {
		return nil, nil, err
	}
	ke.g3 = g3

	g2, err := ke.privateKey.EncryptMasterPublicKey.ScalarBaseMult(rBytes)
	if err != nil {
		return nil, nil, err
	}
	ke.g2 = g2

	if !ke.genSignature {
		return ke.secret, nil, nil
	}

	return ke.secret, ke.sign(true, 0x82), nil
}

// RepondKeyExchange when responder receive rA, for responder's step B1-B7
func (ke *KeyExchange) RepondKeyExchange(rand io.Reader, hid byte, rA *bn256.G1) (*bn256.G1, []byte, error) {
	r, err := randomScalar(rand)
	if err != nil {
		return nil, nil, err
	}
	return respondKeyExchange(ke, hid, r, rA)
}

// ConfirmResponder for initiator's step A5-A7
func (ke *KeyExchange) ConfirmResponder(rB *bn256.G1, sB []byte) ([]byte, []byte, error) {
	if !rB.IsOnCurve() {
		return nil, nil, errors.New("sm9: invalid responder's ephemeral public key")
	}
	// step 5
	ke.peerSecret = rB
	g1, err := ke.privateKey.EncryptMasterPublicKey.ScalarBaseMult(ke.r.Bytes(orderNat))
	if err != nil {
		return nil, nil, err
	}
	ke.g1 = g1
	ke.g2 = bn256.Pair(ke.peerSecret, ke.privateKey.PrivateKey)
	ke.g3 = &bn256.GT{}
	g3, err := bn256.ScalarMultGT(ke.g2, ke.r.Bytes(orderNat))
	if err != nil {
		return nil, nil, err
	}
	ke.g3 = g3
	// step 6, verify signature
	if len(sB) > 0 {
		signature := ke.sign(false, 0x82)
		if goSubtle.ConstantTimeCompare(signature, sB) != 1 {
			return nil, nil, errors.New("sm9: invalid responder's signature")
		}
	}
	key, err := ke.generateSharedKey(false)
	if err != nil {
		return nil, nil, err
	}
	if !ke.genSignature {
		return key, nil, nil
	}
	return key, ke.sign(false, 0x83), nil
}

// ConfirmInitiator for responder's step B8
func (ke *KeyExchange) ConfirmInitiator(s1 []byte) ([]byte, error) {
	if s1 != nil {
		buffer := ke.sign(true, 0x83)
		if goSubtle.ConstantTimeCompare(buffer, s1) != 1 {
			return nil, errors.New("sm9: invalid initiator's signature")
		}
	}
	return ke.generateSharedKey(true)
}
