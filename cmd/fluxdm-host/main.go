// Command fluxdm-host is the native messaging host used by the browser
// extension: it returns the bridge port/token and launches FluxDM if needed.
package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"fluxdm/internal/bridge"
)

func dataDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base, _ = os.UserHomeDir()
	}
	return filepath.Join(base, "FluxDM")
}

type message struct {
	Type string `json:"type"`
}

func main() {
	for {
		var msg message
		if err := readMessage(os.Stdin, &msg); err != nil {
			return // browser closed the port
		}
		switch msg.Type {
		case "hello", "launch":
			d, err := ensureApp(msg.Type == "launch")
			if err != nil {
				writeMessage(os.Stdout, map[string]any{"ok": false, "error": err.Error()})
				continue
			}
			writeMessage(os.Stdout, map[string]any{"ok": true, "port": d.Port, "token": d.Token, "version": d.Version})
		default:
			writeMessage(os.Stdout, map[string]any{"ok": false, "error": "message inconnu"})
		}
	}
}

func ensureApp(show bool) (*bridge.Discovery, error) {
	dir := dataDir()
	if d, err := bridge.ReadDiscovery(dir); err == nil && bridge.Alive(d) {
		if show {
			_ = launch(d.Exe, false)
		}
		return d, nil
	}
	exe := appExe()
	if d, err := bridge.ReadDiscovery(dir); err == nil && d.Exe != "" {
		if _, err := os.Stat(d.Exe); err == nil {
			exe = d.Exe
		}
	}
	if exe == "" {
		return nil, errors.New("FluxDM est introuvable")
	}
	if err := launch(exe, !show); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		if d, err := bridge.ReadDiscovery(dir); err == nil && bridge.Alive(d) {
			return d, nil
		}
	}
	return nil, errors.New("FluxDM ne répond pas")
}

// appExe finds the FluxDM executable next to this host.
func appExe() string {
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	name := "fluxdm"
	if runtime.GOOS == "windows" {
		name = "fluxdm.exe"
	}
	p := filepath.Join(filepath.Dir(self), name)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func readMessage(r io.Reader, v any) error {
	var n uint32
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		return err
	}
	if n > 1<<20 {
		return errors.New("message trop grand")
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	return json.Unmarshal(buf, v)
}

func writeMessage(w io.Writer, v any) {
	raw, _ := json.Marshal(v)
	_ = binary.Write(w, binary.LittleEndian, uint32(len(raw)))
	_, _ = w.Write(raw)
}
