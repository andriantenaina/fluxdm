package scheduler

import (
	"context"
	"net/http"
	"os/exec"
	"runtime"
)

// powerCommand returns the OS command for sleep / hibernate / shutdown.
func powerCommand(action string) *exec.Cmd {
	switch runtime.GOOS {
	case "windows":
		switch action {
		case "shutdown":
			// 60 s warning; the user can cancel with "shutdown /a"
			return exec.Command("shutdown", "/s", "/t", "60", "/c", "FluxDM : téléchargements terminés")
		case "hibernate":
			return exec.Command("shutdown", "/h")
		case "sleep":
			return exec.Command("rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0")
		}
	case "darwin":
		switch action {
		case "shutdown":
			return exec.Command("osascript", "-e", `tell application "System Events" to shut down`)
		case "sleep", "hibernate":
			return exec.Command("pmset", "sleepnow")
		}
	default:
		switch action {
		case "shutdown":
			return exec.Command("systemctl", "poweroff")
		case "hibernate":
			return exec.Command("systemctl", "hibernate")
		case "sleep":
			return exec.Command("systemctl", "suspend")
		}
	}
	return nil
}

func newHead(ctx context.Context, rawURL string, headers map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	return req, nil
}
