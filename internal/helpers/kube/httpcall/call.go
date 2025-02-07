package httpcall

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	finopsdatatypes "github.com/krateoplatformops/finops-data-types/api/v1"
)

type Options struct {
	API      *finopsdatatypes.API
	Endpoint *Endpoint
	DS       map[string]any
}

func Do(ctx context.Context, client *http.Client, opts Options) (*http.Response, error) {
	uri := strings.TrimSuffix(opts.Endpoint.ServerURL, "/")
	if len(opts.API.Path) > 0 {
		uri = fmt.Sprintf("%s/%s", uri, strings.TrimPrefix(opts.API.Path, "/"))
	}

	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	verb := opts.API.Verb

	var body io.Reader
	if len(opts.API.Payload) > 0 {
		body = strings.NewReader(opts.API.Payload)
	}

	req, err := http.NewRequestWithContext(ctx, verb, u.String(), body)
	if err != nil {
		return nil, err
	}

	if len(opts.API.Headers) > 0 {
		for _, el := range opts.API.Headers {
			idx := strings.Index(el, ":")
			if idx <= 0 {
				continue
			}
			req.Header.Set(el[:idx], el[idx+1:])
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// Determine whether the request `content-type` includes a
// server-acceptable mime-type
//
// Failure should yield an HTTP 415 (`http.StatusUnsupportedMediaType`)
func hasContentType(r *http.Response, mimetype string) bool {
	contentType := r.Header.Get("Content-type")
	if contentType == "" {
		return mimetype == "application/octet-stream"
	}

	for _, v := range strings.Split(contentType, ",") {
		t, _, err := mime.ParseMediaType(v)
		if err != nil {
			break
		}
		if t == mimetype {
			return true
		}
	}
	return false
}
