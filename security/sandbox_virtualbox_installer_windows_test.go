//go:build windows
// +build windows

package security

import (
	"strings"
	"testing"
)

// TestWindowsSandboxIgnoresTheVirtualBoxInstallerVariables is M26-TMP-012's
// Windows half. VirtualBox's installers set VBOX_INSTALL_PATH (older ones) or
// VBOX_MSI_INSTALL_PATH (the current one) on the machine they install
// VirtualBox on, which is a host that runs guests and not a guest. The first
// was scored as a VirtualBox guest at exactly the threshold, so secure mode
// refused to run on such a host and reported it as VirtualBox at 70.
//
// It runs the real detector once, with both variables set, because the first
// was read inside detectSandboxWindows and nowhere a fixture reaches. What it
// requires holds on any host: no indicator names either variable, and
// VirtualBox is not the answer unless a Guest Additions driver or process is
// among the indicators.
func TestWindowsSandboxIgnoresTheVirtualBoxInstallerVariables(t *testing.T) {
	for _, name := range []string{"VBOX_INSTALL_PATH", "VBOX_MSI_INSTALL_PATH"} {
		t.Setenv(name, `C:\Program Files\Oracle\VirtualBox\`)
	}

	detection, err := detectSandboxWindows()
	if err != nil {
		t.Fatalf("detectSandboxWindows: %v", err)
	}

	guestAdditions := false
	for _, indicator := range detection.Indicators {
		switch indicator {
		case windowsIndicatorFileVboxMouse, windowsIndicatorFileVboxGuest, windowsIndicatorProcVboxTools:
			guestAdditions = true
		}
		if strings.HasPrefix(indicator, "windows:env:vbox") {
			t.Errorf("indicator %q scores a VirtualBox installer variable, which marks the host "+
				"VirtualBox is installed on and not a guest", indicator)
		}
	}
	if detection.Type == windowsSandboxTypeVirtualBox && !guestAdditions {
		t.Errorf("reported %q at confidence %d from %v, with no Guest Additions driver or process among them",
			detection.Type, detection.Confidence, detection.Indicators)
	}
}
