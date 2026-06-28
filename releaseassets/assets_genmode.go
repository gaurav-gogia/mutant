//go:build releaseassetsgen
// +build releaseassetsgen

package releaseassets

import (
	"fmt"
	"strings"
)

func Get(goos, goarch string) ([]byte, error) {
	key := fmt.Sprintf("%s/%s", strings.ToLower(goos), normalizeArch(strings.ToLower(goarch)))
	return nil, fmt.Errorf("unable to generate release mode builds: embedded runtime asset missing for %s (run 'mutant gen --release-assets' and rebuild mutant)", key)
}
