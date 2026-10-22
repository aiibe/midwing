//go:build darwin && production

package clisetup

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Current() (Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Manager{}, err
	}
	executable, err := os.Executable()
	if err != nil {
		return Manager{}, err
	}
	return Manager{Home: home, Executable: executable,
		Roots: []string{"/Applications", filepath.Join(home, "Applications")},
		VerifyBundle: func(bundle string) bool {
			id, err := exec.Command("/usr/libexec/PlistBuddy", "-c", "Print :CFBundleIdentifier", filepath.Join(bundle, "Contents", "Info.plist")).Output()
			return err == nil && strings.TrimSpace(string(id)) == "com.wails.midwing"
		}}, nil
}
