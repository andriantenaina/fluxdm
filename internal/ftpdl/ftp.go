// Package ftpdl downloads files over FTP/FTPS with resume (REST).
package ftpdl

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/jlaffaye/ftp"

	"fluxdm/internal/core"
	"fluxdm/internal/engine"
	"fluxdm/internal/store"
)

type state struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// Runner is the core.Runner for store.KindFTP.
func Runner(ctx context.Context, m *core.Manager, it *store.Item, rep core.Reporter) error {
	u, err := url.Parse(it.URL)
	if err != nil {
		return err
	}
	host := u.Host
	if u.Port() == "" {
		host += ":21"
	}
	opts := []ftp.DialOption{ftp.DialWithContext(ctx), ftp.DialWithTimeout(20 * time.Second)}
	if u.Scheme == "ftps" {
		opts = append(opts, ftp.DialWithExplicitTLS(&tls.Config{ServerName: u.Hostname(), InsecureSkipVerify: m.Settings().InsecureTLS})) //nolint:gosec
	}
	c, err := ftp.Dial(host, opts...)
	if err != nil {
		return err
	}
	defer c.Quit()

	user, pass := "anonymous", "anonymous@"
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	} else {
		s := m.Settings()
		if cr := s.CredFor(u.Hostname()); cr != nil {
			user, pass = cr.User, cr.Password
		}
	}
	if err := c.Login(user, pass); err != nil {
		return err
	}
	remote := u.Path
	size, _ := c.FileSize(remote)

	var st state
	if len(it.State) > 0 {
		_ = json.Unmarshal(it.State, &st)
	}
	if st.Path == "" {
		name := it.FileName
		if name == "" {
			name = engine.SanitizeFileName(path.Base(remote))
		}
		st.Path = engine.UniquePath(it.Dir, name)
	}
	st.Size = size
	rep.Info(filepath.Base(st.Path), st.Path, size)
	if err := os.MkdirAll(filepath.Dir(st.Path), 0o755); err != nil {
		return err
	}
	part := st.Path + engine.PartExt
	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, _ := f.Stat()
	offset := fi.Size()
	if size > 0 && offset > size {
		offset = 0
		_ = f.Truncate(0)
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	resp, err := c.RetrFrom(remote, uint64(offset))
	if err != nil {
		return err
	}
	defer resp.Close()

	done := offset
	lim := m.Limiter()
	buf := make([]byte, 256<<10)
	last := time.Now()
	lastBytes := done
	var speed float64
	for {
		if ctx.Err() != nil {
			rep.SaveState(st, done)
			return ctx.Err()
		}
		n, rerr := resp.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			done += int64(n)
			_ = lim.WaitN(ctx, n)
		}
		if now := time.Now(); now.Sub(last) > 300*time.Millisecond {
			inst := float64(done-lastBytes) / now.Sub(last).Seconds()
			speed = 0.7*speed + 0.3*inst
			last, lastBytes = now, done
			rep.Progress(core.Progress{Size: size, Downloaded: done, Speed: speed, Conns: 1})
			rep.SaveState(st, done)
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				break
			}
			rep.SaveState(st, done)
			return rerr
		}
	}
	f.Close()
	resp.Close()
	if size > 0 && done < size {
		return io.ErrUnexpectedEOF
	}
	final := st.Path
	if _, err := os.Stat(final); err == nil {
		final = engine.UniquePath(filepath.Dir(final), filepath.Base(final))
	}
	if err := os.Rename(part, final); err != nil {
		return err
	}
	rep.Info(filepath.Base(final), final, done)
	return nil
}
