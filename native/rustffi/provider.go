//go:build !cgo || !mutant_rust
// +build !cgo !mutant_rust

package rustffi

type provider interface {
	Invoke(request string) (string, error)
}

func newProvider() provider {
	return &stubProvider{}
}

type stubProvider struct{}

func (s *stubProvider) Invoke(_ string) (string, error) {
	return "", ErrUnavailable
}
