// monitors.go
package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/gotk3/gotk3/gtk"
)

// monitorInfo describes one connected monitor in desktop coordinates.
type monitorInfo struct {
	Name string
	W, H int
	X, Y int
}

// Example line of "xrandr --listmonitors":
//
//	0: +*eDP-1 1920/344x1080/194+0+0  eDP-1
var monitorLineRe = regexp.MustCompile(
	`^\s*\d+:\s+\S+\s+(\d+)/\d+x(\d+)/\d+\+(-?\d+)\+(-?\d+)\s+(\S+)\s*$`,
)

// listMonitors returns the connected monitors, or nil if they cannot be read.
func listMonitors() []monitorInfo {
	out, err := exec.Command("xrandr", "--listmonitors").Output()
	if err != nil {
		return nil
	}

	var result []monitorInfo

	for _, line := range strings.Split(string(out), "\n") {
		match := monitorLineRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		w, err1 := strconv.Atoi(match[1])
		h, err2 := strconv.Atoi(match[2])
		x, err3 := strconv.Atoi(match[3])
		y, err4 := strconv.Atoi(match[4])

		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			continue
		}

		if w < 16 || h < 16 {
			continue
		}

		result = append(result, monitorInfo{
			Name: match[5],
			W:    w,
			H:    h,
			X:    x,
			Y:    y,
		})
	}

	return result
}

// findMonitor looks a monitor up by its output name (for example "HDMI-1").
// It queries xrandr again so a monitor that was unplugged is detected.
func findMonitor(name string) (monitorInfo, bool) {
	for _, mon := range listMonitors() {
		if mon.Name == name {
			return mon, true
		}
	}

	return monitorInfo{}, false
}

// appendMonitorChoices adds one "Monitor: ..." entry per screen to the Screen
// combo. Nothing is added on single-monitor systems, where "Full Screen"
// already records that monitor.
func appendMonitorChoices(combo *gtk.ComboBoxText) {
	monitors := listMonitors()
	if len(monitors) < 2 {
		return
	}

	for _, mon := range monitors {
		combo.Append(
			"monitor:"+mon.Name,
			fmt.Sprintf("Monitor: %s (%dx%d)", mon.Name, mon.W, mon.H),
		)
	}
}
