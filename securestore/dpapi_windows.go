//go:build windows

package securestore

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

type dpapiProtector struct{}

var dpapiEntropy = []byte("RTA Excel Filler cookie store v1")

func newNativeProtector() (DataProtector, error) {
	return dpapiProtector{}, nil
}

func (dpapiProtector) Protect(plaintext []byte) ([]byte, error) {
	return cryptProtect(plaintext, true)
}

func (dpapiProtector) Unprotect(ciphertext []byte) ([]byte, error) {
	return cryptProtect(ciphertext, false)
}

func cryptProtect(input []byte, protect bool) ([]byte, error) {
	var inputBlob windows.DataBlob
	if len(input) > 0 {
		inputBlob.Size = uint32(len(input))
		inputBlob.Data = &input[0]
	}
	entropyBlob := windows.DataBlob{Size: uint32(len(dpapiEntropy)), Data: &dpapiEntropy[0]}
	var outputBlob windows.DataBlob
	var err error
	if protect {
		err = windows.CryptProtectData(&inputBlob, nil, &entropyBlob, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &outputBlob)
	} else {
		err = windows.CryptUnprotectData(&inputBlob, nil, &entropyBlob, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &outputBlob)
	}
	runtime.KeepAlive(input)
	if err != nil {
		if protect {
			return nil, fmt.Errorf("Windows DPAPI protect: %w", err)
		}
		return nil, fmt.Errorf("Windows DPAPI unprotect: %w", err)
	}
	if outputBlob.Data == nil || outputBlob.Size == 0 {
		return []byte{}, nil
	}
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(outputBlob.Data))) }()
	result := make([]byte, int(outputBlob.Size))
	copy(result, unsafe.Slice(outputBlob.Data, int(outputBlob.Size)))
	return result, nil
}
