//go:build releaseassetsgen
// +build releaseassetsgen

package releaseassets

// RuntimeAssetFiles is empty in generation mode to prevent recursive self-embedding.
var RuntimeAssetFiles = map[string]string{}
