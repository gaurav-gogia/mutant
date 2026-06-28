//go:build !releaseassetsgen
// +build !releaseassetsgen

package releaseassets

import (
	"fmt"
	"io/fs"
	"strings"
)

type missingRuntimeFS struct{}

func (missingRuntimeFS) Open(name string) (fs.File, error) {
	return nil, fs.ErrNotExist
}

func Get(goos, goarch string) ([]byte, error) {
	key := fmt.Sprintf("%s/%s", strings.ToLower(goos), normalizeArch(strings.ToLower(goarch)))
	relPath, ok := RuntimeAssetFiles[key]
	if !ok || strings.TrimSpace(relPath) == "" {
		return nil, fmt.Errorf("unable to generate release mode builds: embedded runtime asset missing for %s (run 'mutant gen --release-assets' and rebuild mutant)", key)
	}

	binaryData, err := fs.ReadFile(runtimeAssetFS, relPath)
	if err != nil {
		return nil, fmt.Errorf("unable to generate release mode builds: embedded runtime asset for %s is invalid: %w", key, err)
	}

	return binaryData, nil
}
