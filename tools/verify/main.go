// Command verify exercises controller operations headlessly, for scripted
// evidence. It is not shipped as the app; the UI calls the same methods.
package main

import (
	"flag"
	"fmt"
	"time"

	"codmtune/internal/sys"
	"codmtune/internal/tune"
)

func main() {
	cmd := flag.String("cmd", "", "detect|apply-reg|restore-reg|apply-fov|fov-keys|power-inject|power-restore|tdr-on|tdr-off|mouse-on|mouse-off|tweak-on|tweak-off|timer-on|timer-off|replay-scan|sensitivity-save|sensitivity-restore")
	view := flag.Int("view", 0, "view FOV")
	weapon := flag.Int("weapon", 0, "weapon FOV")
	id := flag.String("id", "", "tweak id")
	flag.Parse()
	c := tune.NewControl()
	if *view > 0 {
		c.SetFOVView(*view)
	}
	if *weapon > 0 {
		c.SetFOVWeapon(*weapon)
	}
	switch *cmd {
	case "detect":
		c.Detect()
		for i := 0; i < 60; i++ {
			exe, _, _, _, _ := c.Game()
			if exe != "" {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
	case "apply-reg":
		c.ApplyRegistry()
	case "restore-reg":
		c.RestoreRegistry()
	case "apply-fov":
		c.ApplyFOV()
	case "exclusive":
		p, err := c.ExclusiveShortcut()
		fmt.Println(p, err)
	case "timeouts":
		t := sys.CurrentTimeouts()
		fmt.Printf("monitor AC/DC %ds/%ds  standby %ds/%ds  hibernate %ds/%ds\n", t.MonitorAC, t.MonitorDC, t.StandbyAC, t.StandbyDC, t.HibernateAC, t.HibernateDC)
	case "fov-keys":
		v, w := c.FOVKeys()
		fmt.Printf("view keys=%d weapon keys=%d\n", len(v), len(w))
		if len(w) > 0 {
			fmt.Printf("weapon[0]=%q\n", w[0])
		}
		if b, ok := c.CurrentWeaponFOVByte(); ok {
			fmt.Printf("current weapon byte=0x%02X\n", b)
		} else {
			fmt.Printf("current weapon: byte read failed\n")
		}
	case "power-inject":
		c.SetModule("power", true)
	case "power-restore":
		c.RestorePower()
	case "tdr-on":
		c.SetTdr(true)
	case "tdr-off":
		c.SetTdr(false)
	case "mouse-on":
		c.SetMouseAccel(true)
	case "mouse-off":
		c.SetMouseAccel(false)
	case "tweak-on":
		c.SetTweak(*id, true)
	case "tweak-off":
		c.SetTweak(*id, false)
	case "timer-on":
		c.SetTimerBoost(true)
	case "timer-off":
		c.SetTimerBoost(false)
	case "timer-probe":
		before := sys.CurrentTimerResolution()
		cur, err := sys.TimerResolution(5000, true)
		time.Sleep(400 * time.Millisecond)
		after := sys.CurrentTimerResolution()
		_, _ = sys.TimerResolution(5000, false)
		fmt.Printf("timer before=%d requested=%d effective=%d released -> %d err=%v\n", before, cur, after, sys.CurrentTimerResolution(), err)
	case "replay-scan":
		r := c.ScanReplays()
		fmt.Printf("replays=%d\n", len(r))
		for _, x := range r {
			fmt.Printf("%s %d\n", x.Path, x.Size)
		}
	case "sensitivity-save":
		n, err := c.SensitivitySnapshot()
		fmt.Printf("saved=%d err=%v\n", n, err)
	case "sensitivity-restore":
		n, err := c.RestoreSensitivity()
		fmt.Printf("restored=%d err=%v\n", n, err)
	default:
		fmt.Println("no command")
		return
	}
	time.Sleep(700 * time.Millisecond)
	for _, line := range c.Logs() {
		fmt.Println(line)
	}
	exe, _, _, running, pid := c.Game()
	fmt.Printf("game=%s running=%v pid=%d\n", exe, running, pid)
}
