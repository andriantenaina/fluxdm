//go:build windows

package engine

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestSparseFile(t *testing.T) {
	data := randomData(t, 2<<20)
	srv := rangeServer(data, 0, nil, nil)
	defer srv.Close()
	d, err := run(t, testConfig(), Snapshot{URL: srv.URL + "/sparse.bin", Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	p := d.Snapshot().Path
	checkFile(t, p, data)
	name, _ := windows.UTF16PtrFromString(p)
	attrs, err := windows.GetFileAttributes(name)
	if err != nil {
		t.Fatal(err)
	}
	if attrs&windows.FILE_ATTRIBUTE_SPARSE_FILE == 0 {
		var fs [64]uint16
		root, _ := windows.UTF16PtrFromString(p[:3])
		_ = windows.GetVolumeInformation(root, nil, 0, nil, nil, nil, &fs[0], uint32(len(fs)))
		if windows.UTF16ToString(fs[:]) != "NTFS" {
			t.Skipf("système de fichiers %q sans fichiers sparse", windows.UTF16ToString(fs[:]))
		}
		t.Error("attribut FILE_ATTRIBUTE_SPARSE_FILE absent")
	}

}
