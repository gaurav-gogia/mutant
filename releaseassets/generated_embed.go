//go:build !releaseassetsgen
// +build !releaseassetsgen

package releaseassets

import (
	"embed"
	"io/fs"
)

//go:embed data/*.bin
var embeddedRuntimeFS embed.FS

var runtimeAssetFS fs.FS = embeddedRuntimeFS
