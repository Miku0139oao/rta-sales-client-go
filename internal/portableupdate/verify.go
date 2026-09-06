package portableupdate

import (
	"crypto/sha256"
	"debug/pe"
	"errors"
	"fmt"
	"io"
)

// Identity must come from platform Authenticode trust validation, not the
// unauthenticated certificate table or PE version strings alone.
type Identity struct {
	Trusted          bool
	PublisherSubject string
	Version          string
}

type IdentityVerifier interface {
	VerifyIdentity(path string) (Identity, error)
}

// VerifyPublisher accepts leaf-certificate rotation, but not publisher changes.
// Trust of BOTH the currently running executable and candidate is mandatory.
func VerifyPublisher(verifier IdentityVerifier, currentPath, stagedPath, expectedVersion string) error {
	if verifier == nil {
		return errors.New("Authenticode verifier unavailable")
	}
	current, err := verifier.VerifyIdentity(currentPath)
	if err != nil {
		return fmt.Errorf("current executable trust: %w", err)
	}
	if !current.Trusted || current.PublisherSubject == "" {
		return errors.New("current executable is not trusted")
	}
	currentVersion, err := ParseVersion(current.Version)
	if err != nil {
		return errors.New("current executable version is unknown")
	}
	expected, err := ParseVersion(expectedVersion)
	if err != nil || !expected.NewerThan(currentVersion) {
		return errors.New("candidate must be a newer stable version")
	}
	candidate, err := verifier.VerifyIdentity(stagedPath)
	if err != nil {
		return fmt.Errorf("candidate trust: %w", err)
	}
	if !candidate.Trusted || candidate.PublisherSubject != current.PublisherSubject {
		return errors.New("candidate is unsigned, untrusted or from a different publisher")
	}
	actual, err := ParseVersion(candidate.Version)
	if err != nil || actual != expected {
		return errors.New("signed candidate version differs from checked release")
	}
	return nil
}

// VerifyImage checks bytes through a caller-held read handle. Production callers
// must hold deny-write/delete handles and validate non-reparse file identity
// across verification and replacement; a path-only check is insufficient.
func VerifyImage(reader io.ReaderAt, size int64, expectedHash [32]byte, machine uint16) error {
	if size <= 0 || size > executableLimit {
		return errors.New("invalid image size")
	}
	hash := sha256.New()
	if n, err := io.Copy(hash, io.NewSectionReader(reader, 0, size)); err != nil || n != size {
		return errors.New("cannot hash entire image")
	}
	var actual [32]byte
	copy(actual[:], hash.Sum(nil))
	if actual != expectedHash {
		return errors.New("staged image changed after download")
	}
	image, err := pe.NewFile(reader)
	if err != nil {
		return fmt.Errorf("invalid PE executable: %w", err)
	}
	defer image.Close()
	if machine != pe.IMAGE_FILE_MACHINE_AMD64 || image.Machine != machine || image.Characteristics&pe.IMAGE_FILE_EXECUTABLE_IMAGE == 0 || image.Characteristics&pe.IMAGE_FILE_DLL != 0 {
		return errors.New("unsupported portable PE architecture/type")
	}
	if _, ok := image.OptionalHeader.(*pe.OptionalHeader64); !ok {
		return errors.New("candidate is not a PE32+ executable")
	}
	return nil
}
