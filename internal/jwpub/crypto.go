// Package jwpub opens JW Library publication files (.jwpub) and decrypts
// their content.
//
// The decryption scheme is not documented by its publisher. The algorithm was
// ported from sws2apps/meeting-schedules-parser, src/common/jwpub_parser.ts
// (https://github.com/sws2apps/meeting-schedules-parser), MIT License,
// Copyright (c) 2025 Scheduling Workbox System. Meeting Media Manager
// (github.com/sircharlo/meeting-media-manager, AGPL-3.0) was checked first: it
// reads JWPUB databases but never decrypts Document.Content, so it could not
// be the source. See README.md, "Créditos y licencias".
package jwpub

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// cardMask is the constant every publication card hash is XORed with.
const cardMask = "11cbb5587e32846d4c26790c633da289f66fe5842a3a585ce1bc3a294af5ada7"

// Card identifies a publication the way the key derivation expects it. The
// values come from the Publication table of the .db inside the JWPUB.
type Card struct {
	MepsLanguageIndex int
	Symbol            string // Publication.Symbol, e.g. "mwb26", "nwtsty"
	Year              int
	IssueTagNumber    int // 0 for undated publications (books, Bibles)
}

// String renders the card as "lang_symbol_year[_issue]". The issue is left
// out when it is zero: nwtsty only decrypts as "1_nwtsty_2026", while every
// periodical needs it ("1_mwb26_2026_20260900").
func (c Card) String() string {
	s := fmt.Sprintf("%d_%s_%d", c.MepsLanguageIndex, c.Symbol, c.Year)
	if c.IssueTagNumber != 0 {
		s += fmt.Sprintf("_%d", c.IssueTagNumber)
	}
	return s
}

// Cipher holds the AES-128 key and IV derived from a publication card.
type Cipher struct {
	Key [16]byte
	IV  [16]byte
}

// NewCipher derives key and IV: SHA-256(card) XOR cardMask, first half is
// the key, second half the IV.
func NewCipher(c Card) Cipher {
	mask, _ := hex.DecodeString(cardMask)
	sum := sha256.Sum256([]byte(c.String()))
	for i := range sum {
		sum[i] ^= mask[i]
	}
	var ci Cipher
	copy(ci.Key[:], sum[:16])
	copy(ci.IV[:], sum[16:])
	return ci
}

// ErrNotEncrypted is returned for blobs that are empty or not a whole number
// of AES blocks.
var ErrNotEncrypted = errors.New("jwpub: el blob no tiene el tamaño de un contenido cifrado")

// Decrypt turns an encrypted content blob (Document.Content, BibleVerse.Content,
// Extract.Content, ...) into the UTF-8 HTML it holds: AES-128-CBC with PKCS#7
// padding, then zlib.
func (ci Cipher) Decrypt(blob []byte) ([]byte, error) {
	if len(blob) == 0 || len(blob)%aes.BlockSize != 0 {
		return nil, ErrNotEncrypted
	}
	block, err := aes.NewCipher(ci.Key[:])
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(blob))
	cipher.NewCBCDecrypter(block, ci.IV[:]).CryptBlocks(plain, blob)
	pad := int(plain[len(plain)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(plain) {
		return nil, fmt.Errorf("jwpub: relleno PKCS#7 inválido (%d): ¿clave equivocada?", pad)
	}
	for _, b := range plain[len(plain)-pad:] {
		if int(b) != pad {
			return nil, errors.New("jwpub: relleno PKCS#7 inconsistente: ¿clave equivocada?")
		}
	}
	zr, err := zlib.NewReader(bytes.NewReader(plain[:len(plain)-pad]))
	if err != nil {
		return nil, fmt.Errorf("jwpub: zlib: %w", err)
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

// DecryptString is Decrypt for callers that want text.
func (ci Cipher) DecryptString(blob []byte) (string, error) {
	b, err := ci.Decrypt(blob)
	return string(b), err
}

// Encrypt is the inverse of Decrypt. The library never writes JWPUB files; it
// exists so tests can build fixtures without shipping publication content.
func (ci Cipher) Encrypt(plain []byte) ([]byte, error) {
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	if _, err := zw.Write(plain); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	data := z.Bytes()
	pad := aes.BlockSize - len(data)%aes.BlockSize
	data = append(data, bytes.Repeat([]byte{byte(pad)}, pad)...)
	block, err := aes.NewCipher(ci.Key[:])
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	cipher.NewCBCEncrypter(block, ci.IV[:]).CryptBlocks(out, data)
	return out, nil
}
