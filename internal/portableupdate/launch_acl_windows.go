//go:build windows

package portableupdate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A read-only/deny-delete open cannot coexist with an existing DELETE-access
// open: Windows checks sharing in both directions. The target ACL boundary is
// therefore mandatory BEFORE dropping the rename handle, not just a hash after
// reopening. No other unprivileged principal may replace the leaf in that gap,
// mutate its contents, change path metadata, or grant itself those rights.
func validateTargetBoundary(target *os.File, targetPath string, guards []*os.File) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	current := user.User.Sid.String()
	parent := filepath.Dir(targetPath)
	found := false
	for _, dir := range guards {
		// p.guards also contains private staging and cwd ancestry; checking these
		// conservatively is intentional. Only the target parent forbids creation.
		immediate := strings.EqualFold(dir.Name(), parent)
		if immediate {
			found = true
		}
		if err := validateLaunchACL(dir, current, true, immediate); err != nil {
			return err
		}
	}
	if !found {
		return errors.New("target parent is not pinned")
	}
	if target != nil {
		return validateLaunchACL(target, current, false, false)
	}
	return nil
}

func validateLaunchACL(file *os.File, current string, directory, immediateParent bool) error {
	sd, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read launch boundary ACL: %w", err)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !trustedLaunchPrincipal(owner.String(), current) {
		return errors.New("unsafe update path owner; use a private user-owned folder")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return errors.New("unsafe update path: missing restrictive DACL")
	}
	header := (*struct {
		Revision, Reserved     byte
		Size, Count, Reserved2 uint16
	})(unsafe.Pointer(acl))
	for index := uint32(0); index < uint32(header.Count); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err = windows.GetAce(acl, index, &ace); err != nil {
			return err
		}
		// An inheritance-only ACE is not effective on this object. Child ACLs are
		// checked separately on their actual handles. Deny ACEs cannot add access;
		// do not try to use them to excuse a dangerous broad allow (fail closed).
		if ace.Header.AceFlags&0x08 != 0 || ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("unsupported launch-boundary ACE; use a private folder")
		}
		principal := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
		if unsafeLaunchGrant(principal, current, uint32(ace.Mask), directory, immediateParent) {
			return fmt.Errorf("unsafe update path permissions at %s (principal %s, mask %#x); use a private user-owned folder", file.Name(), principal, uint32(ace.Mask))
		}
	}
	return nil
}
func trustedLaunchPrincipal(sid, current string) bool {
	return sid == current || sid == "S-1-5-18" /* SYSTEM */ || sid == "S-1-5-32-544" /* Administrators */ ||
		sid == "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464" // Windows TrustedInstaller (often owns the volume root)
}
func unsafeLaunchGrant(sid, current string, mask uint32, directory, immediateParent bool) bool {
	if trustedLaunchPrincipal(sid, current) {
		return false
	}
	// Permit only read/list/execute/synchronise/read-control rights for others.
	// Ancestor creation rights do not let someone alter an existing pinned child;
	// the immediate target parent must additionally exclude FILE_ADD_FILE and
	// FILE_ADD_SUBDIRECTORY to protect the vacant target during the rename pair.
	allowed := uint32(0x80000000 | 0x20000000 | 0x00100000 | 0x00020000 | 0x01 | 0x08 | 0x20 | 0x80)
	if directory && !immediateParent {
		allowed |= 0x02 | 0x04
	}
	// This rejects DELETE, FILE_DELETE_CHILD, WRITE_DAC, WRITE_OWNER, writes,
	// write attributes/EA, generic write/all and unfamiliar grant bits.
	return mask & ^allowed != 0
}
