package portableupdate

import (
	"bytes"
	"crypto/sha256"
	"debug/pe"
	"encoding/binary"
	"testing"
)

// Synthetic PE headers only; no executable is launched or replaced.
func imageFixture(machine uint16, dll bool) []byte {
	var out bytes.Buffer
	dos := make([]byte, 128)
	copy(dos, "MZ")
	binary.LittleEndian.PutUint32(dos[0x3c:], 128)
	out.Write(dos)
	out.WriteString("PE\x00\x00")
	characteristics := uint16(pe.IMAGE_FILE_EXECUTABLE_IMAGE)
	if dll {
		characteristics |= pe.IMAGE_FILE_DLL
	}
	header := pe.FileHeader{Machine: machine, SizeOfOptionalHeader: uint16(binary.Size(pe.OptionalHeader64{})), Characteristics: characteristics}
	_ = binary.Write(&out, binary.LittleEndian, header)
	_ = binary.Write(&out, binary.LittleEndian, pe.OptionalHeader64{Magic: 0x20b, NumberOfRvaAndSizes: 16})
	return out.Bytes()
}
func TestVerifyImage(t *testing.T) {
	image := imageFixture(pe.IMAGE_FILE_MACHINE_AMD64, false)
	hash := sha256.Sum256(image)
	if err := VerifyImage(bytes.NewReader(image), int64(len(image)), hash, pe.IMAGE_FILE_MACHINE_AMD64); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		image []byte
		hash  [32]byte
	}{
		{"tamper", append(append([]byte{}, image...), 0), hash},
		{"wrong architecture", imageFixture(pe.IMAGE_FILE_MACHINE_I386, false), sha256.Sum256(imageFixture(pe.IMAGE_FILE_MACHINE_I386, false))},
		{"dll", imageFixture(pe.IMAGE_FILE_MACHINE_AMD64, true), sha256.Sum256(imageFixture(pe.IMAGE_FILE_MACHINE_AMD64, true))},
		{"not PE", []byte("test"), sha256.Sum256([]byte("test"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if VerifyImage(bytes.NewReader(tc.image), int64(len(tc.image)), tc.hash, pe.IMAGE_FILE_MACHINE_AMD64) == nil {
				t.Fatal("accepted unsafe image")
			}
		})
	}
	if VerifyImage(bytes.NewReader(image), int64(len(image)+1), hash, pe.IMAGE_FILE_MACHINE_AMD64) == nil {
		t.Fatal("accepted short reader")
	}
}
