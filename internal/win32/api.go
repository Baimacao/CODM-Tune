// Package win32 is the native Windows layer: syscall bindings, a GDI+ canvas
// that implements ui.Painter, and the window/message loop. No CGO.
package win32

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	psapi    = syscall.NewLazyDLL("psapi.dll")
	iphlpapi = syscall.NewLazyDLL("iphlpapi.dll")
	pdh      = syscall.NewLazyDLL("pdh.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	winmm    = syscall.NewLazyDLL("winmm.dll")
)

func p(dll *syscall.LazyDLL, name string) *syscall.LazyProc { return dll.NewProc(name) }

var (
	pRegisterClassExW              = p(user32, "RegisterClassExW")
	pCreateWindowExW               = p(user32, "CreateWindowExW")
	pDefWindowProcW                = p(user32, "DefWindowProcW")
	pDestroyWindow                 = p(user32, "DestroyWindow")
	pShowWindow                    = p(user32, "ShowWindow")
	pUpdateWindow                  = p(user32, "UpdateWindow")
	pGetMessageW                   = p(user32, "GetMessageW")
	pTranslateMessage              = p(user32, "TranslateMessage")
	pDispatchMessageW              = p(user32, "DispatchMessageW")
	pPostQuitMessage               = p(user32, "PostQuitMessage")
	pPostMessageW                  = p(user32, "PostMessageW")
	pSetTimer                      = p(user32, "SetTimer")
	pKillTimer                     = p(user32, "KillTimer")
	pGetClientRect                 = p(user32, "GetClientRect")
	pAdjustWindowRectEx            = p(user32, "AdjustWindowRectEx")
	pBeginPaint                    = p(user32, "BeginPaint")
	pEndPaint                      = p(user32, "EndPaint")
	pGetDC                         = p(user32, "GetDC")
	pReleaseDC                     = p(user32, "ReleaseDC")
	pReleaseCapture                = p(user32, "ReleaseCapture")
	pLoadCursorW                   = p(user32, "LoadCursorW")
	pLoadIconW                     = p(user32, "LoadIconW")
	pSetCursor                     = p(user32, "SetCursor")
	pSetForegroundWindow           = p(user32, "SetForegroundWindow")
	pSetProcessDPIAware            = p(user32, "SetProcessDPIAware")
	pSetProcessDpiAwarenessContext = p(user32, "SetProcessDpiAwarenessContext")
	pGetDpiForSystem               = p(user32, "GetDpiForSystem")
	pCreatePopupMenu               = p(user32, "CreatePopupMenu")
	pAppendMenuW                   = p(user32, "AppendMenuW")
	pTrackPopupMenu                = p(user32, "TrackPopupMenu")
	pDestroyMenu                   = p(user32, "DestroyMenu")
	pGetCursorPos                  = p(user32, "GetCursorPos")
	pScreenToClient                = p(user32, "ScreenToClient")
	pSetWindowTextW                = p(user32, "SetWindowTextW")
	pMessageBoxW                   = p(user32, "MessageBoxW")
	pFindWindowW                   = p(user32, "FindWindowW")
	pIsIconic                      = p(user32, "IsIconic")
	pIsWindowVisible               = p(user32, "IsWindowVisible")
	pSetWindowPos                  = p(user32, "SetWindowPos")
	pGetSystemMetrics              = p(user32, "GetSystemMetrics")
	pGetWindowRect                 = p(user32, "GetWindowRect")
	pGetWindowLongPtrW             = p(user32, "GetWindowLongPtrW")
	pSetWindowLongPtrW             = p(user32, "SetWindowLongPtrW")
	pSetLayeredWindowAttributes    = p(user32, "SetLayeredWindowAttributes")
	pLoadImageW                    = p(user32, "LoadImageW")
	pSendMessageW                  = p(user32, "SendMessageW")
	pSystemParametersInfoW         = p(user32, "SystemParametersInfoW")

	pCreateMutexW      = p(kernel32, "CreateMutexW")
	pGetLastError      = p(kernel32, "GetLastError")
	pGetModuleHandleW  = p(kernel32, "GetModuleHandleW")
	pGetCurrentProcess = p(kernel32, "GetCurrentProcess")
	pSetPriorityClass  = p(kernel32, "SetPriorityClass")
	pCloseHandle       = p(kernel32, "CloseHandle")
	pGetTickCount64    = p(kernel32, "GetTickCount64")

	pCreateCompatibleDC = p(gdi32, "CreateCompatibleDC")
	pDeleteDC           = p(gdi32, "DeleteDC")
	pCreateDIBSection   = p(gdi32, "CreateDIBSection")
	pSelectObject       = p(gdi32, "SelectObject")
	pDeleteObject       = p(gdi32, "DeleteObject")
	pBitBlt             = p(gdi32, "BitBlt")

	pShellNotifyIconW = p(shell32, "Shell_NotifyIconW")
	pShellExecuteW    = p(shell32, "ShellExecuteW")

	pDwmSetWindowAttribute = p(dwmapi, "DwmSetWindowAttribute")

	pRegOpenKeyExW    = p(advapi32, "RegOpenKeyExW")
	pRegCloseKey      = p(advapi32, "RegCloseKey")
	pRegQueryValueExW = p(advapi32, "RegQueryValueExW")
	pRegSetValueExW   = p(advapi32, "RegSetValueExW")
	pRegDeleteValueW  = p(advapi32, "RegDeleteValueW")
	pRegEnumValueW    = p(advapi32, "RegEnumValueW")
	pRegQueryInfoKeyW = p(advapi32, "RegQueryInfoKeyW")
	pRegCreateKeyExW  = p(advapi32, "RegCreateKeyExW")
	pRegDeleteKeyW    = p(advapi32, "RegDeleteKeyW")

	pGetProcessMemoryInfo = p(psapi, "GetProcessMemoryInfo")
	pEmptyWorkingSet      = p(psapi, "EmptyWorkingSet")
	pGetExtendedTcpTable  = p(iphlpapi, "GetExtendedTcpTable")

	pPdhOpenQueryW                = p(pdh, "PdhOpenQueryW")
	pPdhAddEnglishCounterW        = p(pdh, "PdhAddEnglishCounterW")
	pPdhCollectQueryData          = p(pdh, "PdhCollectQueryData")
	pPdhGetFormattedCounterArrayW = p(pdh, "PdhGetFormattedCounterArrayW")
	pPdhCloseQuery                = p(pdh, "PdhCloseQuery")

	pCoInitializeEx   = p(ole32, "CoInitializeEx")
	pCoCreateInstance = p(ole32, "CoCreateInstance")
	pCoUninitialize   = p(ole32, "CoUninitialize")

	pTimeBeginPeriod = p(winmm, "timeBeginPeriod")
	pTimeEndPeriod   = p(winmm, "timeEndPeriod")
)

const (
	WM_DESTROY     = 0x0002
	WM_SIZE        = 0x0005
	WM_PAINT       = 0x000F
	WM_CLOSE       = 0x0010
	WM_ERASEBKGND  = 0x0014
	WM_KEYDOWN     = 0x0100
	WM_KEYUP       = 0x0101
	WM_SYSKEYDOWN  = 0x0104
	WM_SYSKEYUP    = 0x0105
	WM_MOUSEMOVE   = 0x0200
	WM_LBUTTONDOWN = 0x0201
	WM_LBUTTONUP   = 0x0202
	WM_MOUSEWHEEL  = 0x020A
	WM_TIMER       = 0x0113
	WM_APP         = 0x8000

	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_CAPTION          = 0x00C00000
	WS_SYSMENU          = 0x00080000
	WS_POPUP            = 0x80000000
	WS_THICKFRAME       = 0x00040000
	WS_MINIMIZEBOX      = 0x00020000
	WS_MAXIMIZEBOX      = 0x00010000
	WS_VISIBLE          = 0x10000000
	CW_USEDEFAULT       = 0x80000000

	SW_HIDE     = 0
	SW_SHOW     = 5
	SW_RESTORE  = 9
	SW_MINIMIZE = 6

	COLOR_WINDOW    = 5
	IDC_ARROW       = 32512
	IDI_APPLICATION = 32512
	SIZE_MINIMIZED  = 1

	CS_HREDRAW = 1
	CS_VREDRAW = 2

	NIM_ADD         = 0
	NIM_MODIFY      = 1
	NIM_DELETE      = 2
	NIF_MESSAGE     = 1
	NIF_ICON        = 2
	NIF_TIP         = 4
	WM_LBUTTONUP_   = 0x0202
	WM_RBUTTONUP_   = 0x0205
	MF_STRING       = 0
	MF_CHECKED      = 0x00000008
	MF_SEPARATOR    = 0x800
	MF_DEFAULT      = 0x1000
	MF_DISABLED     = 0x00000002
	MF_GRAYED       = 0x00000001
	TPM_RETURNCMD   = 0x0100
	TPM_RIGHTBUTTON = 0x0002

	SRCCOPY        = 0x00CC0020
	DIB_RGB_COLORS = 0
	BI_RGB         = 0

	ERROR_ALREADY_EXISTS        = 183
	BELOW_NORMAL_PRIORITY_CLASS = 0x00004000
)

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type MSG struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
	Private uint32
}

type POINT struct{ X, Y int32 }

type RECT struct{ Left, Top, Right, Bottom int32 }

type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type BITMAPINFOHEADER struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type BITMAPINFO struct {
	BmiHeader BITMAPINFOHEADER
	BmiColors [1]uint32
}

type GdiplusStartupInput struct {
	GdiplusVersion           uint32
	DebugEventCallback       uintptr
	SuppressBackgroundThread int32
	SuppressExternalCodecs   int32
}

type RectF struct{ X, Y, Width, Height float32 }

type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type NOTIFYICONDATAW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         GUID
	HBalloonIcon     uintptr
}

func loword(v uintptr) int32  { return int32(int16(v & 0xffff)) }
func hiword(v uintptr) int32  { return int32(int16((v >> 16) & 0xffff)) }
func loWord32(v uint32) int32 { return int32(int16(v & 0xffff)) }
func hiWord32(v uint32) int32 { return int32(int16((v >> 16) & 0xffff)) }

var _ = unsafe.Pointer(nil)
