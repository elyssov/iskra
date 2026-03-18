package firewall

import (
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// EnsureFirewallRule checks if Windows Firewall has an exception for the app
// and adds inbound+outbound rules if missing. No-op on non-Windows.
// Failures are logged but never fatal — user may not have admin privileges.
func EnsureFirewallRule(appName string) {
	if runtime.GOOS != "windows" {
		return
	}

	// Check if rule already exists
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

	log.Printf("[FW] Adding firewall rules for '%s' (%s)", appName, exePath)

	// Add inbound rule
	inCmd := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+appName,
		"dir=in",
		"action=allow",
		"program="+exePath,
		"enable=yes",
		"profile=private",
	)
	if out, err := inCmd.CombinedOutput(); err != nil {
		log.Printf("[FW] Failed to add inbound rule (need admin?): %v — %s", err, strings.TrimSpace(string(out)))
	} else {
		log.Printf("[FW] Inbound rule added successfully")
	}

	// Add outbound rule
	outCmd := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+appName,
		"dir=out",
		"action=allow",
		"program="+exePath,
		"enable=yes",
		"profile=private",
	)
	if out, err := outCmd.CombinedOutput(); err != nil {
		log.Printf("[FW] Failed to add outbound rule (need admin?): %v — %s", err, strings.TrimSpace(string(out)))
	} else {
		log.Printf("[FW] Outbound rule added successfully")
	}
}
