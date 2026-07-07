// SECURITY-CRITICAL (vault/CLAUDE.md). Pure format layer for blob
// format v1 and the wrapped-key envelope (ADR-0021). The exported
// functions are pure — invoke twice, byte-compare — and their exact
// bytes are pinned by docs/vectors/blob.json.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

const (
	blobVersion1 = 0x01
	keySize      = 32
	nonceSize    = 12
	tagSize      = 16
	wrappedSize  = nonceSize + keySize + tagSize // 60 bytes, 120 hex chars
)

// gcmFor builds the AEAD. Both constructors are infallible for a
// fixed 32-byte key and standard GCM parameters.
func gcmFor(key [keySize]byte) cipher.AEAD {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		panic("vault: aes.NewCipher rejected a 32-byte key")
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		panic("vault: cipher.NewGCM rejected standard parameters")
	}
	return g
}

// EncodeBlobV1 produces the v1 blob file:
// 0x01 || nonce(12) || AES-256-GCM ct||tag, AAD = the 1-byte header,
// so every stored byte that is not ciphertext is AEAD-authenticated.
// dek and nonce must be fresh random draws for every blob; fixed
// values appear only in golden-vector tests — a nonce reused under
// the same key breaks GCM.
func EncodeBlobV1(dek [keySize]byte, nonce [nonceSize]byte, plaintext []byte) []byte {
	out := make([]byte, 0, 1+nonceSize+len(plaintext)+tagSize)
	out = append(out, blobVersion1)
	out = append(out, nonce[:]...)
	return gcmFor(dek).Seal(out, nonce[:], plaintext, []byte{blobVersion1})
}

// decodeBlobV1 opens a v1 blob file. Every failure — length, version,
// authentication — collapses to ErrCorrupt; nothing from the crypto
// layer escapes into errors (vault/CLAUDE.md).
func decodeBlobV1(dek [keySize]byte, file []byte) ([]byte, error) {
	if len(file) < 1+nonceSize+tagSize || file[0] != blobVersion1 {
		return nil, ErrCorrupt
	}
	pt, err := gcmFor(dek).Open(nil, file[1:1+nonceSize], file[1+nonceSize:], []byte{blobVersion1})
	if err != nil {
		return nil, ErrCorrupt
	}
	return pt, nil
}

// BlobAddress is the content address: sha256 over the ENTIRE stored
// file, lowercase hex (RFC-0002 §6 "hash of ciphertext" as pinned by
// ADR-0021).
func BlobAddress(file []byte) kernel.Hash {
	sum := sha256.Sum256(file)
	return hex.EncodeToString(sum[:])
}

// WrapKey seals key under kek: nonce(12) || ct(32)||tag(16). The AAD
// binds the wrap to its context — raw run id bytes for run keys, the
// 64-char hex blob address for DEKs — so envelopes cannot be spliced
// across runs or entries. The nonce must be a fresh random draw for
// every wrap; fixed nonces appear only in golden-vector tests — a
// nonce reused under the same KEK breaks GCM.
func WrapKey(kek [keySize]byte, nonce [nonceSize]byte, key [keySize]byte, aad []byte) []byte {
	out := make([]byte, 0, wrappedSize)
	out = append(out, nonce[:]...)
	return gcmFor(kek).Seal(out, nonce[:], key[:], aad)
}

func unwrapKey(kek [keySize]byte, wrapped, aad []byte) ([keySize]byte, error) {
	var key [keySize]byte
	if len(wrapped) != wrappedSize {
		return key, ErrCorrupt
	}
	pt, err := gcmFor(kek).Open(nil, wrapped[:nonceSize], wrapped[nonceSize:], aad)
	if err != nil {
		return key, ErrCorrupt
	}
	copy(key[:], pt)
	wipe(pt)
	return key, nil
}

func newKey(rand io.Reader) ([keySize]byte, error) {
	var k [keySize]byte
	if _, err := io.ReadFull(rand, k[:]); err != nil {
		return k, fmt.Errorf("vault: entropy source failed: %w", err)
	}
	return k, nil
}

func newNonce(rand io.Reader) ([nonceSize]byte, error) {
	var n [nonceSize]byte
	if _, err := io.ReadFull(rand, n[:]); err != nil {
		return n, fmt.Errorf("vault: entropy source failed: %w", err)
	}
	return n, nil
}

// wipe zeroes key material. Best-effort only: the GC and AES key
// schedules may retain copies (ADR-0021 Stage-1 caveat).
func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
