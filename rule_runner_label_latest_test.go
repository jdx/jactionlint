package jactionlint

import "testing"

// TestLatestRunnerLabelsArePinned records what the moving "-latest" labels mean. When GitHub moves a label this test fails
// until the table in rule_runner_label.go (the constants above defaultRunnerOSCompats and latestRunnerImages) and the
// table below are both updated, which is how a stale mapping gets noticed.
//
// Last verified against the sources on 2026-10-13:
//   - windows-latest is Windows Server 2025 (windows-2025 and windows-2025-vs2026 are one image).
//   - macos-latest is macOS 26 (arm64), macos-latest-large is macOS 26 Intel, macos-latest-xlarge is macOS 26 xlarge.
//   - ubuntu-latest is Ubuntu 24.04 until the rollout of 2026-10-19 to 2026-11-19 moves it to Ubuntu 26.04, so both are
//     accepted. After the rollout it is Ubuntu 26.04 only.
//   - windows-11-arm and windows-11-vs2026-arm are announced to become one image (actions/runner-images#14602,
//     due 2026-09-30) but the README and the image releases of 2026-10-05 still list two. They stay distinct until
//     the move is confirmed.
func TestLatestRunnerLabelsArePinned(t *testing.T) {
	for label, want := range map[string]runnerOSCompat{
		"ubuntu-latest":          compatUbuntu2404 | compatUbuntu2604,
		"ubuntu-latest-4-cores":  compatUbuntu2404 | compatUbuntu2604,
		"ubuntu-latest-8-cores":  compatUbuntu2404 | compatUbuntu2604,
		"ubuntu-latest-16-cores": compatUbuntu2404 | compatUbuntu2604,
		"windows-latest":         compatWindows2025,
		"windows-latest-8-cores": compatWindows2025,
		"windows-2025":           compatWindows2025,
		"windows-2025-vs2026":    compatWindows2025,
		"macos-latest":           compatMacOS260,
		"macos-latest-large":     compatMacOS260L,
		"macos-latest-xlarge":    compatMacOS260XL,
		"windows-11-arm":         compatWindows11Arm,
		"windows-11-vs2026-arm":  compatWindows11VS2026Arm,
	} {
		if got := defaultRunnerOSCompats[label]; got != want {
			t.Errorf("%s: compatibility is %b, the last verified mapping is %b", label, got, want)
		}
	}

	// The suggestion of the mutable-runner-label rule names an image the label is on
	for os, want := range map[string]string{"ubuntu": "ubuntu-24.04", "windows": "windows-2025", "macos": "macos-26"} {
		img, ok := latestRunnerImages[os]
		if !ok || img.pin != want {
			t.Errorf("%s: the pin is %q, the last verified one is %q", os, img.pin, want)
			continue
		}
		if defaultRunnerOSCompats[img.pin]&defaultRunnerOSCompats[os+"-latest"] == 0 {
			t.Errorf("%s: the pin %q is not an image of %s-latest", os, img.pin, os)
		}
	}
	if latestRunnerImages["ubuntu"].rollout == "" {
		t.Error("ubuntu-latest has a rollout in the last verified mapping")
	}
	if latestRunnerImages["windows"].rollout != "" || latestRunnerImages["macos"].rollout != "" {
		t.Error("windows-latest and macos-latest have no rollout in the last verified mapping")
	}
}
