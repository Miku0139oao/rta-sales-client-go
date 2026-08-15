//go:build !windows

package securestore

func newNativeCredentialStore() (CredentialStore, error) {
	return nil, ErrUnsupported
}
