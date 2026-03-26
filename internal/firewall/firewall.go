package firewall

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// EnsureFirewallRule checks if Windows Firewall has an exception for the app
// and adds inbound+outbound rules if missing. Silent on failure — never blocks startup.
// No-op on non-Windows.
func EnsureFirewallRule(appName string) {
	if runtime.GOOS != "windows" {
		return
	}

	// Check if rule already exists (doesn't need admin)
	check := exec.Command("netsh", "advfirewall", "firewall", "show", "rule", "name="+appName)
	out, err := check.CombinedOutput()
	if err == nil && !strings.Contains(string(out), "No rules match") {
		return // rule exists
	}

	exePath, err := os.Executable()
	if err != nil {
		return
	}

	// Try to add directly (works if already admin)
	addRule := func(dir string) {
		exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
			"name="+appName, "dir="+dir, "action=allow",
			"program="+exePath, "enable=yes", "profile=private",
		).Run()
	}
	addRule("in")
	addRule("out")
	// If it fails (not admin), that's fine — app works without firewall rules on most setups
}
