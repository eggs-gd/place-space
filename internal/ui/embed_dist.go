//go:build ui

package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

// Assets is the interface packed into the executable.
func Assets() fs.FS {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
