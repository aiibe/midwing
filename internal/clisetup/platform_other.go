//go:build !darwin || !production

package clisetup

func Current() (Manager, error) {
	return Manager{Unavailable: "CLI setup is available in the installed macOS release app. Development builds do not create CLI links."}, nil
}
