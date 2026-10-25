package win32

import "runtime/debug"

// TrimWorkingSet asks Windows to page out the process working set. It is a
// maintenance action, not a fix for a leak: callers should also stop creating
// new large surfaces while the app is idle.
func TrimWorkingSet() {
	debug.FreeOSMemory()
	_, _, _ = pEmptyWorkingSet.Call(^uintptr(0))
}
