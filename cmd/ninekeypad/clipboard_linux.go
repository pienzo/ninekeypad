package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// copyToClipboard hands the text to the desktop's copy tool (Wayland or X11). Under sudo
// the tool runs as the desktop user, whose clipboard it is.
func copyToClipboard(text string) error {
	tools := [][]string{
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
	}
	for _, t := range tools {
		if _, err := exec.LookPath(t[0]); err != nil {
			continue
		}
		cmd := asDesktopUser(t)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return errors.New("no copy tool found (wl-copy, xclip or xsel)")
}

func asDesktopUser(argv []string) *exec.Cmd {
	user, uid := os.Getenv("SUDO_USER"), os.Getenv("SUDO_UID")
	if os.Geteuid() != 0 || user == "" || uid == "" {
		return exec.Command(argv[0], argv[1:]...)
	}
	env := []string{"XDG_RUNTIME_DIR=/run/user/" + uid}
	for _, k := range []string{"DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY"} {
		if v := os.Getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	if os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("DISPLAY") == "" {
		env = append(env, "DISPLAY=:0") // sudo usually drops these; :0 is the common default
	}
	args := append([]string{"-u", user, "env"}, append(env, argv...)...)
	return exec.Command("sudo", args...)
}
