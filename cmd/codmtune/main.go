package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"time"

	"codmtune/internal/appui"
	"codmtune/internal/crosshair"
	"codmtune/internal/hud"
	"codmtune/internal/sys"
	"codmtune/internal/win32"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			dir, _ := os.UserConfigDir()
			dir = filepath.Join(dir, "CODMTune")
			_ = os.MkdirAll(dir, 0o755)
			path := filepath.Join(dir, "crash-"+time.Now().Format("20060102-150405")+".txt")
			_ = os.WriteFile(path, []byte(fmt.Sprintf("panic: %v\n\n%s", r, debug.Stack())), 0o644)
			sys.MessageBox("CODM Tune 崩溃", "已写入日志：\r\n"+path)
		}
	}()
	// A CPU profile is opt-in through the environment so a shipped build costs
	// nothing: profiling itself adds overhead, and the whole point is to
	// measure a build that is otherwise identical.
	if path := os.Getenv("CODMTUNE_CPUPROFILE"); path != "" {
		secs := 30
		if v, err := strconv.Atoi(os.Getenv("CODMTUNE_CPUPROFILE_SEC")); err == nil && v >= 5 && v <= 300 {
			secs = v
		}
		if f, err := os.Create(path); err == nil {
			_ = pprof.StartCPUProfile(f)
			go func() {
				time.Sleep(time.Duration(secs) * time.Second)
				pprof.StopCPUProfile()
				_ = f.Close()
				// The profile covers a fixed window; exiting afterwards keeps
				// the probe bounded and never leaves profiling on in real use.
				os.Exit(0)
			}()
		}
	}
	if len(os.Args) > 1 && os.Args[1] == "--crosshair" {
		if err := crosshair.RunChild(); err != nil {
			fmt.Fprintln(os.Stderr, "crosshair:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--overlay" {
		if err := hud.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "hud:", err)
			os.Exit(1)
		}
		return
	}
	if win32.ActivateExistingInstance() {
		return
	}
	if !sys.IsElevated() && os.Getenv("CODMTUNE_DEV") != "1" {
		if err := sys.RelaunchElevated(); err != nil {
			sys.MessageBox("CODM Tune", "该工具写入电源方案与 TdrDelay 需要管理员权限。\r\n"+err.Error())
			os.Exit(1)
		}
		return
	}
	if err := appui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "codmtune:", err)
		os.Exit(1)
	}
}
