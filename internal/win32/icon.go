package win32

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

//go:embed app.ico
var appICO []byte

var gAppIcon uintptr

// loadAppIcon extracts the embedded .ico once and returns an HICON. The icon
// is generated from the turbine-rifle silhouette by tools/icon.
func loadAppIcon() uintptr {
	if gAppIcon != 0 {
		return gAppIcon
	}
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "CODMTune")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "app.ico")
	if st, err := os.Stat(path); err != nil || st.Size() != int64(len(appICO)) {
		_ = os.WriteFile(path, appICO, 0o644)
	}
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0
	}
	h, _, e := pLoadImageW.Call(0, uintptr(unsafe.Pointer(p)), 1 /*IMAGE_ICON*/, 0, 0, 0x0010 /*LR_LOADFROMFILE*/)
	if h == 0 {
		fmt.Println("icon:", e)
	}
	gAppIcon = h
	return h
}
