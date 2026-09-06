//go:build windows

package portableupdate

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WindowsIdentityVerifier uses WinVerifyTrust's verified primary signer chain,
// not a certificate independently extracted from an unverified PE table.
// Revocation is fail-closed using cached chain data only: no UI or unbounded
// network retrieval is permitted. Machines lacking revocation data may reject
// otherwise valid releases and must refresh Windows trust data manually.
type WindowsIdentityVerifier struct{}

var trustDLL = windows.NewLazySystemDLL("wintrust.dll")
var providerData = trustDLL.NewProc("WTHelperProvDataFromStateData")
var providerSigner = trustDLL.NewProc("WTHelperGetProvSignerFromChain")
var providerCert = trustDLL.NewProc("WTHelperGetProvCertFromChain")

func (WindowsIdentityVerifier) VerifyIdentity(path string) (Identity, error) {
	guards, err := guardDirectories(filepath.Dir(path))
	if err != nil {
		return Identity{}, err
	}
	defer closeFiles(guards)
	locked, err := openLocked(path, false)
	if err != nil {
		return Identity{}, err
	}
	defer locked.Close()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Identity{}, err
	}
	file := windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: name, File: windows.Handle(locked.Fd())}
	data := windows.WinTrustData{Size: uint32(unsafe.Sizeof(windows.WinTrustData{})), UIChoice: windows.WTD_UI_NONE,
		RevocationChecks: windows.WTD_REVOKE_WHOLECHAIN, UnionChoice: windows.WTD_CHOICE_FILE,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&file), StateAction: windows.WTD_STATEACTION_VERIFY,
		ProvFlags: windows.WTD_CACHE_ONLY_URL_RETRIEVAL | windows.WTD_REVOCATION_CHECK_CHAIN_EXCLUDE_ROOT}
	err = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &data)
	defer func() {
		data.StateAction = windows.WTD_STATEACTION_CLOSE
		_ = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &data)
	}()
	if err != nil {
		return Identity{}, fmt.Errorf("Authenticode trust rejected: %w", err)
	}
	p, _, _ := providerData.Call(uintptr(data.StateData))
	if p == 0 {
		return Identity{}, errors.New("missing verified provider")
	}
	signer, _, _ := providerSigner.Call(p, 0, 0, 0)
	if signer == 0 {
		return Identity{}, errors.New("missing verified signer")
	}
	cert, _, _ := providerCert.Call(signer, 0)
	if cert == 0 {
		return Identity{}, errors.New("missing verified signer certificate")
	}
	// CRYPT_PROVIDER_CERT's first two members (Windows SDK wintrust.h).
	var prefix struct {
		Size uint32
		Cert *windows.CertContext
	}
	// Copy the documented prefix from WinTrust-owned memory without converting
	// an untracked uintptr into a Go pointer (also safe under checkptr/race).
	var copied uintptr
	if err := windows.ReadProcessMemory(windows.CurrentProcess(), cert, (*byte)(unsafe.Pointer(&prefix)), unsafe.Sizeof(prefix), &copied); err != nil || copied != unsafe.Sizeof(prefix) {
		return Identity{}, errors.New("cannot read verified certificate context")
	}
	if prefix.Cert == nil {
		return Identity{}, errors.New("empty signer certificate")
	}
	format := uint32(3) // CERT_X500_NAME_STR
	n := windows.CertGetNameString(prefix.Cert, windows.CERT_NAME_RDN_TYPE, 0, unsafe.Pointer(&format), nil, 0)
	if n <= 1 || n > 32768 {
		return Identity{}, errors.New("invalid publisher subject")
	}
	text := make([]uint16, n)
	if windows.CertGetNameString(prefix.Cert, windows.CERT_NAME_RDN_TYPE, 0, unsafe.Pointer(&format), &text[0], n) != n {
		return Identity{}, errors.New("cannot read publisher subject")
	}
	version, err := numericFileVersion(path)
	runtime.KeepAlive(file)
	runtime.KeepAlive(locked)
	if err != nil {
		return Identity{}, err
	}
	return Identity{Trusted: true, PublisherSubject: windows.UTF16ToString(text), Version: version}, nil
}

func numericFileVersion(path string) (string, error) {
	var unused windows.Handle
	size, err := windows.GetFileVersionInfoSize(path, &unused)
	if err != nil {
		return "", err
	}
	if size == 0 || size > 1<<20 {
		return "", errors.New("invalid version resource size")
	}
	buf := make([]byte, size)
	if err = windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return "", err
	}
	var info *windows.VS_FIXEDFILEINFO
	var length uint32
	if err = windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\`, unsafe.Pointer(&info), &length); err != nil {
		return "", err
	}
	if info == nil || length < uint32(unsafe.Sizeof(*info)) || info.Signature != 0xfeef04bd {
		return "", errors.New("invalid fixed version resource")
	}
	if info.FileVersionMS != info.ProductVersionMS || info.FileVersionLS != info.ProductVersionLS || info.FileVersionLS&0xffff != 0 || info.FileFlags&info.FileFlagsMask&0x2b != 0 {
		return "", errors.New("PE version is not a stable three-component release")
	}
	version := fmt.Sprintf("%d.%d.%d", info.FileVersionMS>>16, info.FileVersionMS&0xffff, info.FileVersionLS>>16)
	runtime.KeepAlive(buf)
	return version, nil
}
