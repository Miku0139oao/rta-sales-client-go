//go:build windows

package portableupdate

import (
	"encoding/binary"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Creates only a test-owned empty-directory junction using the documented
// REPARSE_DATA_BUFFER mount-point layout; no shell or elevated tool is used.
func makeSandboxJunction(path, target string) error {
	if err := os.Mkdir(path, 0700); err != nil {
		return err
	}
	name, _ := windows.UTF16PtrFromString(path)
	h, err := windows.CreateFile(name, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	substitute, _ := windows.UTF16FromString(`\??\` + target)
	printable, _ := windows.UTF16FromString(target)
	data := make([]byte, 16+2*(len(substitute)+len(printable)))
	binary.LittleEndian.PutUint32(data, windows.IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(data[4:], uint16(len(data)-8))
	binary.LittleEndian.PutUint16(data[10:], uint16(2*(len(substitute)-1)))
	binary.LittleEndian.PutUint16(data[12:], uint16(2*len(substitute)))
	binary.LittleEndian.PutUint16(data[14:], uint16(2*(len(printable)-1)))
	units := unsafe.Slice((*uint16)(unsafe.Pointer(&data[16])), len(substitute)+len(printable))
	copy(units, substitute)
	copy(units[len(substitute):], printable)
	var returned uint32
	return windows.DeviceIoControl(h, windows.FSCTL_SET_REPARSE_POINT, &data[0], uint32(len(data)), nil, 0, &returned, nil)
}
