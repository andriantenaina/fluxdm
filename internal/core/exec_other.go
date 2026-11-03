//go:build !windows

package core

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}

// HideWindow is exported for other packages (yt-dlp, ffmpeg...).
func HideWindow(cmd *exec.Cmd) { hideWindow(cmd) }
