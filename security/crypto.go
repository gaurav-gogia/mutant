package security

import (
	"math/rand"
)

func XOR(data []byte, length int) []byte {
	key := randByte(int64(length))
	for i := range data {
		data[i] ^= key
	}
	return data
}

func XOROne(instruction byte, length int) byte {
	key := randByte(int64(length))
	return instruction ^ key
}

func randByte(seed int64) byte {
	src := rand.NewSource(seed)
	newrand := rand.New(src)
	number := newrand.Int()
	return byte(number)
}
