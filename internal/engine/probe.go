package engine

import (
	"context"
	"net/http"
	"strconv"
)

// Info describes a remote file before downloading it.
type Info struct {
	URL         string `json:"url"`
	FinalURL    string `json:"finalUrl"`
	FileName    string `json:"fileName"`
	Size        int64  `json:"size"` // -1 unknown
	Resumable   bool   `json:"resumable"`
	ContentType string `json:"contentType"`
}

// Probe fetches file information with a 1-byte range request.
func Probe(ctx context.Context, client *http.Client, cfg Config, rawURL string, headers map[string]string) (*Info, error) {
	cfg.normalize()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", cfg.UserAgent)
	}
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Range", "bytes=0-0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Status: resp.Status}
	}
	info := &Info{
		URL:         rawURL,
		FinalURL:    resp.Request.URL.String(),
		ContentType: resp.Header.Get("Content-Type"),
		Size:        -1,
	}
	if resp.StatusCode == http.StatusPartialContent {
		if m := contentRangeRe.FindStringSubmatch(resp.Header.Get("Content-Range")); m != nil && m[3] != "*" {
			info.Size, _ = strconv.ParseInt(m[3], 10, 64)
			info.Resumable = info.Size > 0
		}
	} else if resp.ContentLength >= 0 {
		info.Size = resp.ContentLength
	}
	info.FileName = FileNameFromResponse(info.FinalURL, resp.Header.Get("Content-Disposition"), info.ContentType)
	return info, nil
}
