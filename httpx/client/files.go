package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"time"
)

// MultipartFile describes a local regular file. The SDK owns and closes its temporary file handles.
// Filename defaults to the path's base name; no local directory is sent to the server.
type MultipartFile struct{ Field, Path, Filename, ContentType string }

// Multipart streams form fields/files without buffering whole files. Streaming bodies are never automatically retried.
func (c *Client) Multipart(ctx context.Context, url string, fields map[string]string, files []MultipartFile, output any) error {
	for _, f := range files {
		if f.Field == "" || f.Path == "" {
			return errors.New("http client: multipart file field and path required")
		}
		info, e := os.Stat(f.Path)
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return errors.New("http client: multipart requires a regular file")
		}
		if f.ContentType != "" {
			if _, _, e = mime.ParseMediaType(f.ContentType); e != nil {
				return errors.New("http client: invalid multipart MIME type")
			}
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	form := multipart.NewWriter(writer)
	contentType := form.FormDataContentType()
	done := make(chan struct{})
	go func() {
		defer close(done)
		var result error
		defer func() {
			if result != nil {
				_ = writer.CloseWithError(result)
			} else {
				_ = writer.Close()
			}
		}()
		for k, v := range fields {
			if result = form.WriteField(k, v); result != nil {
				return
			}
		}
		for _, spec := range files {
			if result = ctx.Err(); result != nil {
				return
			}
			f, e := os.Open(spec.Path)
			if e != nil {
				result = e
				return
			}
			name := spec.Filename
			if name == "" {
				name = filepath.Base(spec.Path)
			}
			headers := textproto.MIMEHeader{}
			headers.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": spec.Field, "filename": name}))
			typ := spec.ContentType
			if typ == "" {
				typ = "application/octet-stream"
			}
			headers.Set("Content-Type", typ)
			part, e := form.CreatePart(headers)
			if e == nil {
				_, e = io.Copy(part, f)
			}
			closeErr := f.Close()
			if e != nil {
				result = e
				return
			}
			if closeErr != nil {
				result = closeErr
				return
			}
		}
		result = form.Close()
	}()
	defer func() { cancel(); _ = reader.Close(); <-done }()
	r, e := http.NewRequestWithContext(ctx, http.MethodPost, url, reader)
	if e != nil {
		return e
	}
	r.Header.Set("Content-Type", contentType)
	res, e := c.Do(r)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &StatusError{Status: res.StatusCode, RetryAfter: parseRetryAfter(res.Header.Get("Retry-After"), time.Now())}
	}
	if output == nil {
		_, e = io.Copy(io.Discard, res.Body)
		return e
	}
	d := json.NewDecoder(res.Body)
	if e = d.Decode(output); e != nil {
		return e
	}
	var extra any
	if e = d.Decode(&extra); e != io.EOF {
		return errors.New("http client: trailing JSON response")
	}
	return nil
}

// Download streams a successful response into destination with the configured response size bound.
// A failed download may have partially written destination; use DownloadFile for atomic replacement.
func (c *Client) Download(ctx context.Context, url string, destination io.Writer) (int64, error) {
	if destination == nil {
		return 0, errors.New("http client: download destination required")
	}
	r, e := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if e != nil {
		return 0, e
	}
	res, e := c.Do(r)
	if e != nil {
		return 0, e
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return 0, &StatusError{Status: res.StatusCode, RetryAfter: parseRetryAfter(res.Header.Get("Retry-After"), c.config.Clock.Now())}
	}
	return io.Copy(destination, res.Body)
}

// DownloadFile writes a sibling temporary file and atomically replaces path only after a complete successful response.
// Files use mode 0600. Failed/canceled downloads remove the temporary file and preserve the previous destination.
func (c *Client) DownloadFile(ctx context.Context, url, path string) (err error) {
	if path == "" {
		return errors.New("http client: download path required")
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".gpk-download-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer func() { _ = f.Close(); _ = os.Remove(name) }()
	if _, e = c.Download(ctx, url, f); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(name, path)
}
