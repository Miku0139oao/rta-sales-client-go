//go:build windows

package securestore

import (
	"errors"
	"fmt"

	"github.com/danieljoos/wincred"
)

const credentialTargetPrefix = "RTA Excel Filler/profile/"

type windowsCredentialStore struct{}

func newNativeCredentialStore(string) (CredentialStore, error) {
	return windowsCredentialStore{}, nil
}

func (windowsCredentialStore) Get(profileID string) (Credential, error) {
	if err := validateProfileID(profileID); err != nil {
		return Credential{}, err
	}
	stored, err := wincred.GetGenericCredential(credentialTargetPrefix + profileID)
	if errors.Is(err, wincred.ErrElementNotFound) {
		return Credential{}, ErrNotFound
	}
	if err != nil {
		return Credential{}, fmt.Errorf("read Windows credentials: %w", err)
	}
	return Credential{Account: stored.UserName, Password: string(stored.CredentialBlob)}, nil
}

func (windowsCredentialStore) Put(profileID string, value Credential) error {
	if err := validateProfileID(profileID); err != nil {
		return err
	}
	stored := wincred.NewGenericCredential(credentialTargetPrefix + profileID)
	stored.UserName = value.Account
	stored.CredentialBlob = []byte(value.Password)
	stored.Persist = wincred.PersistLocalMachine
	if err := stored.Write(); err != nil {
		return fmt.Errorf("write Windows credentials: %w", err)
	}
	return nil
}

func (windowsCredentialStore) Delete(profileID string) error {
	if err := validateProfileID(profileID); err != nil {
		return err
	}
	stored, err := wincred.GetGenericCredential(credentialTargetPrefix + profileID)
	if errors.Is(err, wincred.ErrElementNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Windows credentials for deletion: %w", err)
	}
	if err := stored.Delete(); err != nil && !errors.Is(err, wincred.ErrElementNotFound) {
		return fmt.Errorf("delete Windows credentials: %w", err)
	}
	return nil
}
