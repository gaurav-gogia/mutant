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
	profileCode := security.ResolveProtectionProfileCode()
	canary := deriveStandaloneTailCanary(bytecode, checksum[:])
	provenance := security.DeriveStandaloneProvenance(bytecode, checksum[:], profileCode)

	trailer := make([]byte, 0, security.StandaloneTailV3Size)
	trailer = append(trailer, []byte(security.StandaloneTailMarker)...)
	trailer = append(trailer, security.StandaloneTailV3)

	lenBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(lenBuf, uint64(len(bytecode)))
	trailer = append(trailer, lenBuf...)
	trailer = append(trailer, checksum[:]...)
	trailer = append(trailer, canary...)
	trailer = append(trailer, profileCode)
	trailer = append(trailer, provenance[:]...)

	releaseBinary := make([]byte, 0, len(runtimeBinary)+len(bytecode)+len(trailer))
	releaseBinary = append(releaseBinary, runtimeBinary...)
	releaseBinary = append(releaseBinary, bytecode...)
	releaseBinary = append(releaseBinary, trailer...)

	return releaseBinary
}

func deriveStandaloneTailCanary(payload []byte, checksum []byte) []byte {
	seed := make([]byte, 0, len(payload)+len(checksum)+len(security.StandaloneTailMarker)+1)
	seed = append(seed, payload...)
	seed = append(seed, checksum...)
	seed = append(seed, []byte(security.StandaloneTailMarker)...)
	seed = append(seed, security.StandaloneTailV2)
	digest := sha256.Sum256(seed)
	return digest[:8]
}
