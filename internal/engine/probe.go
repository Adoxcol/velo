package engine

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Info describes a link before it's downloaded (for the Add Download dialog).
type Info struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`      // -1 if unknown
	Resumable bool   `json:"resumable"` // server supports byte ranges
	Category  string `json:"category"`
	Kind      string `json:"kind"` // "", "hls" or "dash"
}

// Probe asks the server about r.URL with a one-byte range request.
func (e *Engine) Probe(ctx context.Context, r Request) (Info, error) {
	// Security: Validate that the URL uses an HTTP or HTTPS scheme before probing
	// to prevent requests to unsupported protocols or local file schemes.
	u := strings.ToLower(r.URL)
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return Info{}, errors.New("invalid URL scheme")
	}
	if ok, kind := Downloadable(r.URL); ok && kind != "" {
		name := videoName(Request{URL: r.URL, Title: r.Title}) + ".mp4"
		return Info{Name: name, Size: -1, Resumable: true, Category: "Video", Kind: kind}, nil
	}
	resp, err := e.openFirst(ctx, &r)
	if err != nil {
		return Info{}, err
	}
	defer resp.Body.Close()
	io.CopyN(io.Discard, resp.Body, 1)
	info := Info{Size: -1}
	switch resp.StatusCode {
	case http.StatusPartialContent:
		if cr := resp.Header.Get("Content-Range"); strings.Contains(cr, "/") {
			if n, err := strconv.ParseInt(cr[strings.LastIndex(cr, "/")+1:], 10, 64); err == nil {
				info.Size, info.Resumable = n, true
			}
		}
	case http.StatusOK:
		info.Size = resp.ContentLength
	default:
		return Info{}, refusedError(resp)
	}
	info.Name = fileName(r, resp)
	info.Category = CategoryOf(info.Name)
	return info, nil
}
