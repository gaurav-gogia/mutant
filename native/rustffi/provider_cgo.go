//go:build cgo && mutant_rust
// +build cgo,mutant_rust

package rustffi

/*
#cgo windows LDFLAGS: -lmutant_rust
#cgo linux LDFLAGS: -lmutant_rust
#cgo darwin LDFLAGS: -lmutant_rust

#include <stdlib.h>

char* mutant_rust_probe(const char* request);
void mutant_rust_free(char* ptr);
*/
import "C"

import (
	"errors"
	"unsafe"
)

type cgoProvider struct{}

func newProvider() provider {
	return &cgoProvider{}
}

func (p *cgoProvider) Invoke(request string) (string, error) {
	cRequest := C.CString(request)
	defer C.free(unsafe.Pointer(cRequest))

	response := C.mutant_rust_probe(cRequest)
	if response == nil {
		return "", errors.New("rust probe returned nil response")
	}
	defer C.mutant_rust_free(response)

	return C.GoString(response), nil
}
