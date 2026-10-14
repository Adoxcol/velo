package engine

import (
	"context"
	"fmt"
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
	if ok, kind := Downloadable(r.URL); ok && kind != "" {
		name := videoName(Request{URL: r.URL, Title: r.Title}) + ".mp4"
		return Info{Name: name, Size: -1, Resumable: true, Category: "Video", Kind: kind}, nil
	}
	req, err := e.newReq(ctx, r)
	if err != nil {
		return Info{}, err
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := e.Client.Do(req)
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
		return Info{}, fmt.Errorf("server returned %s", resp.Status)
	}
	info.Name = fileName(r, resp)
	info.Category = CategoryOf(info.Name)
	return info, nil
}
