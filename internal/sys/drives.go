package sys

import "syscall"

var (
	pGetLogicalDrives = kernel32DLL.NewProc("GetLogicalDrives")
	pGetDriveTypeW    = kernel32DLL.NewProc("GetDriveTypeW")
)

// FixedDrives lists ready local fixed disks, D first because CODM installs
// there by convention.
func FixedDrives() []string {
	bits, _, _ := pGetLogicalDrives.Call()
	var out []string
	order := []string{"D", "E", "F", "G", "C"}
	seen := map[string]bool{}
	check := func(letter string) {
		if seen[letter] {
			return
		}
		seen[letter] = true
		p, _ := syscall.UTF16PtrFromString(letter + `:\`)
		t, _, _ := pGetDriveTypeW.Call(uintptr(unsafePointer(p)))
		if t == 3 { // DRIVE_FIXED
			out = append(out, letter+`:`) 
		}
	}
	for _, l := range order {
		idx := int(l[0] - 'A')
		if bits&(1<<uint(idx)) != 0 {
			check(l)
		}
	}
	for i := 0; i < 26; i++ {
		if bits&(1<<uint(i)) != 0 {
			check(string(rune('A' + i)))
		}
	}
	return out
}
