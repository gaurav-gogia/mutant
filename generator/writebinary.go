package generator

import (
	"crypto/sha256"
	"encoding/binary"
	"mutant/releaseassets"
	"mutant/security"
	"os"
)

func writeBinaryRelease(dstpath, goos, goarch string, bytecode []byte) error {
	runtimeBinary, err := releaseassets.Get(goos, goarch)
	if err != nil {
		return err
	}

	releaseBinary := appendReleasePayload(runtimeBinary, bytecode)
	return os.WriteFile(dstpath, releaseBinary, 0755)
}

func appendReleasePayload(runtimeBinary []byte, bytecode []byte) []byte {
	checksum := sha256.Sum256(bytecode)
	trailer := make([]byte, 0, security.StandaloneTailSize)
	trailer = append(trailer, []byte(security.StandaloneTailMarker)...)
	trailer = append(trailer, security.StandaloneTailV1)

	lenBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(lenBuf, uint64(len(bytecode)))
	trailer = append(trailer, lenBuf...)
	trailer = append(trailer, checksum[:]...)

	releaseBinary := make([]byte, 0, len(runtimeBinary)+len(bytecode)+len(trailer))
	releaseBinary = append(releaseBinary, runtimeBinary...)
	releaseBinary = append(releaseBinary, bytecode...)
	releaseBinary = append(releaseBinary, trailer...)

	return releaseBinary
}
