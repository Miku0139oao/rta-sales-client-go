//go:build !windows

package securestore

func newNativeProtector() (DataProtector, error) {
	return nil, ErrUnsupported
}
