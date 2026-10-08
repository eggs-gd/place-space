// Package id generates opaque identifiers.
package id

import (
	"crypto/rand"
	"encoding/hex"
)

// New returns a random 128-bit hex id.
func New() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
