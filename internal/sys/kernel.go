package sys

import "syscall"

var (
	kernel32DLL = syscall.NewLazyDLL("kernel32.dll")
	psapiDLL    = syscall.NewLazyDLL("psapi.dll")
	iphlpapiDLL = syscall.NewLazyDLL("iphlpapi.dll")
	pdhDLL      = syscall.NewLazyDLL("pdh.dll")
	ole32DLL    = syscall.NewLazyDLL("ole32.dll")
	shell32DLL  = syscall.NewLazyDLL("shell32.dll")
)

var (
	pGetProcessAffinityMask           = kernel32DLL.NewProc("GetProcessAffinityMask")
	pSetProcessAffinityMask           = kernel32DLL.NewProc("SetProcessAffinityMask")
	pGetProcessMemoryInfo             = psapiDLL.NewProc("GetProcessMemoryInfo")
	pGetLogicalProcessorInformationEx = kernel32DLL.NewProc("GetLogicalProcessorInformationEx")
	pGetExtendedTcpTable              = iphlpapiDLL.NewProc("GetExtendedTcpTable")
)
