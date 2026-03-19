package firewall

import (
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// EnsureFirewallRule checks if Windows Firewall has an exception for the app
// and adds inbound+outbound rules if missing via UAC elevation.
// No-op on non-Windows. Failures are logged but never fatal.
func EnsureFirewallRule(appName string) {
	if runtime.GOOS != "windows" {
		return
	}

	// Check if rule already exists (doesn't need admin)
	check := exec.Command("netsh", "advfirewall", "firewall", "show", "rule", "name="+appName)
	out, err := check.CombinedOutput()
	if err == nil && !strings.Contains(string(out), "No rules match") {
		log.Printf("[FW] Firewall rule '%s' already exists", appName)
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		log.Printf("[FW] Cannot determine executable path: %v", err)
		return
	}

	log.Printf("[FW] Adding firewall rules for '%s' via UAC...", appName)

	// Build netsh commands
	inRule := "netsh advfirewall firewall add rule name='" + appName + "' dir=in action=allow program='" + exePath + "' enable=yes profile=private"
	outRule := "netsh advfirewall firewall add rule name='" + appName + "' dir=out action=allow program='" + exePath + "' enable=yes profile=private"
	script := inRule + "; " + outRule

	// Use PowerShell Start-Process with -Verb RunAs to trigger UAC dialog
	// The entire command must be a single -Command string for proper parsing
	psCommand := "Start-Process -FilePath powershell -ArgumentList '-Command \"" + script + "\"' -Verb RunAs -Wait"

	cmd := exec.Command("powershell", "-NoProfile", "-Command", psCommand)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("[FW] UAC elevation failed or was declined: %v — %s", err, strings.TrimSpace(string(out)))
	} else {
		log.Printf("[FW] Firewall rules added successfully via UAC")
	}
}
