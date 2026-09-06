//go:build windows

package portableupdate

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type fileID struct{ Volume, High, Low uint32 }

func idOf(f *os.File) (fileID, error) {
	var i windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &i); err != nil {
		return fileID{}, err
	}
	if i.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fileID{}, errors.New("reparse points are not update paths")
	}
	if i.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 && i.NumberOfLinks != 1 {
		return fileID{}, errors.New("hard-linked executable rejected")
	}
	return fileID{i.VolumeSerialNumber, i.FileIndexHigh, i.FileIndexLow}, nil
}
func localPath(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || len(filepath.VolumeName(path)) != 2 || strings.Contains(path[2:], ":") || strings.ContainsAny(path, "\x00\r\n") {
		return errors.New("update paths must be canonical local drive paths without alternate streams")
	}
	for _, part := range strings.Split(path[3:], `\`) {
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return errors.New("ambiguous Windows path")
		}
	}
	return nil
}
func openLocked(path string, rename bool) (*os.File, error) {
	if err := localPath(path); err != nil {
		return nil, err
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	access := uint32(windows.GENERIC_READ)
	if rename {
		access |= windows.DELETE
	}
	h, err := windows.CreateFile(p, access, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), path)
	if _, err = idOf(f); err != nil {
		f.Close()
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("update file must be regular")
	}
	return f, nil
}
func hashFile(f *os.File) ([32]byte, error) {
	var sum [32]byte
	stat, err := f.Stat()
	if err != nil {
		return sum, err
	}
	if stat.Size() <= 0 || stat.Size() > executableLimit {
		return sum, errors.New("invalid executable size")
	}
	h := sha256.New()
	_, err = io.Copy(h, io.NewSectionReader(f, 0, stat.Size()))
	copy(sum[:], h.Sum(nil))
	return sum, err
}

// Directory handles deny DELETE on every ancestor, keeping path ancestry stable.
// They share WRITE so Windows may create children. Setting reparse metadata or
// changing ACLs as the same user/admin is outside our threat boundary.
func guardDirectories(path string) ([]*os.File, error) {
	if err := localPath(path); err != nil {
		return nil, err
	}
	root, _ := windows.UTF16PtrFromString(filepath.VolumeName(path) + `\`)
	driveType := windows.GetDriveType(root)
	if driveType != windows.DRIVE_FIXED && driveType != windows.DRIVE_REMOVABLE {
		return nil, errors.New("updates require a local filesystem volume")
	}
	var guards []*os.File
	parts := []string{}
	for p := path; ; p = filepath.Dir(p) {
		parts = append(parts, p)
		if filepath.Dir(p) == p {
			break
		}
	}
	for i := len(parts) - 1; i >= 0; i-- {
		name, _ := windows.UTF16PtrFromString(parts[i])
		h, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			closeFiles(guards)
			return nil, err
		}
		f := os.NewFile(uintptr(h), parts[i])
		guards = append(guards, f)
		if _, err := idOf(f); err != nil {
			closeFiles(guards)
			return nil, err
		}
		info, err := f.Stat()
		if err != nil || !info.IsDir() {
			closeFiles(guards)
			return nil, errors.New("invalid update directory")
		}
	}
	return guards, nil
}
func closeFiles(files []*os.File) {
	for _, f := range files {
		if f != nil {
			_ = f.Close()
		}
	}
}
func nonce() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
func privateDirectory(parent string) (string, error) {
	guards, err := guardDirectories(parent)
	if err != nil {
		return "", err
	}
	defer closeFiles(guards)
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	sd, err := windows.SecurityDescriptorFromString("O:" + user.User.Sid.String() + "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return "", err
	}
	token, err := nonce()
	if err != nil {
		return "", err
	}
	path := filepath.Join(parent, ".rta-update-"+token)
	p, _ := windows.UTF16PtrFromString(path)
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	if err = windows.CreateDirectory(p, &sa); err != nil {
		return "", err
	}
	return path, nil
}
func validatePrivateDirectory(f *os.File) error {
	sd, err := windows.GetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil || !owner.Equals(user.User.Sid) {
		return errors.New("staging owner differs from current user")
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("staging ACL must be protected")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return errors.New("missing private staging DACL")
	}
	// ACL header is the documented eight-byte Windows ACL structure.
	header := (*struct {
		Revision, Reserved     byte
		Size, Count, Reserved2 uint16
	})(unsafe.Pointer(acl))
	if header.Count != 2 {
		return errors.New("unexpected staging permissions")
	}
	seen := map[string]bool{}
	for i := uint32(0); i < 2; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err = windows.GetAce(acl, i, &ace); err != nil {
			return err
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask != 0x1f01ff /* FILE_ALL_ACCESS */ {
			return errors.New("unexpected staging ACE")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
		if sid != user.User.Sid.String() && sid != "S-1-5-18" {
			return errors.New("staging accessible by another principal")
		}
		seen[sid] = true
	}
	if !seen[user.User.Sid.String()] || !seen["S-1-5-18"] {
		return errors.New("incomplete staging permissions")
	}
	return nil
}
func copyExclusive(source *os.File, path string) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	stat, err := source.Stat()
	if err == nil {
		_, err = io.Copy(out, io.NewSectionReader(source, 0, stat.Size()))
	}
	if err == nil {
		err = out.Sync()
	}
	return errors.Join(err, out.Close())
}

// Rename by the SAME handle that owns DELETE access. FILE_SHARE_READ denies
// other writers/deleters but does not prevent this handle's own rename. A new
// rename handle is acquired after path-based trust checks and bytes/ID rechecked.
func renameHandle(f *os.File, destination string, replace bool) error {
	if err := localPath(destination); err != nil {
		return err
	}
	name, err := windows.UTF16FromString(destination)
	if err != nil {
		return err
	}
	name = name[:len(name)-1]
	type renameInfo struct {
		Replace uint32
		Root    windows.Handle
		Length  uint32
		Name    uint16
	}
	offset := int(unsafe.Offsetof(renameInfo{}.Name))
	buf := make([]byte, offset+len(name)*2)
	info := (*renameInfo)(unsafe.Pointer(&buf[0]))
	if replace {
		info.Replace = 1
	}
	info.Length = uint32(len(name) * 2)
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(&buf[offset])), len(name)), name)
	if err = windows.SetFileInformationByHandle(windows.Handle(f.Fd()), windows.FileRenameInfo, &buf[0], uint32(len(buf))); err != nil {
		return fmt.Errorf("atomic rename: %w", err)
	}
	return nil
}
