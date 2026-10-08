//go:build !ui

package ui

import (
	"embed"
	"io/fs"
)

//go:embed fallback
var embedded embed.FS

// Assets is the unpacked development page.
func Assets() fs.FS {
	sub, err := fs.Sub(embedded, "fallback")
	if err != nil {
		panic(err)
	}
	return sub
}
