package aichat

import (
	"bytes"
	"errors"
	"io"
	"net/http"
)

const visionResponseBytes = 256 << 10

var errVisionResponseLimit = errors.New("vision response exceeds byte limit")

// Keep model statistics outside this transport, so a size rejection is counted
// as a failed exchange. This bound also covers non-200 and decompressed bodies.
func boundedVisionClient(client *http.Client) *http.Client {
	return boundedModelResponseClient(client, visionResponseBytes, errVisionResponseLimit)
}

func boundedModelResponseClient(client *http.Client, limit int, limitError error) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	copy := *client
	base := copy.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copy.Transport = &modelResponseTransport{base: base, limit: limit, limitError: limitError}
	return &copy
}

type modelResponseTransport struct {
	base       http.RoundTripper
	limit      int
	limitError error
}

func (t *modelResponseTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
func (t *modelResponseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rsp, err := t.base.RoundTrip(req)
	if err != nil || rsp == nil {
		return rsp, err
	}
	if rsp.Body == nil || rsp.Body == http.NoBody {
		return rsp, nil
	}
	if rsp.ContentLength > int64(t.limit) {
		rsp.Body.Close()
		return nil, t.limitError
	}
	if rsp.ContentLength < 0 {
		// These clients consume non-streaming model replies. Verify unknown-length
		// transfers to EOF within the same byte bound before handing them to a JSON
		// decoder, which may otherwise stop before observing the transfer's EOF.
		body := rsp.Body
		data, readErr := io.ReadAll(&modelResponseBody{ReadCloser: body, remaining: t.limit, limitError: t.limitError})
		closeErr := body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		rsp.Body = io.NopCloser(bytes.NewReader(data))
		rsp.ContentLength = int64(len(data))
		return rsp, nil
	}
	rsp.Body = &modelResponseBody{ReadCloser: rsp.Body, remaining: t.limit, limitError: t.limitError}
	return rsp, nil
}

type modelResponseBody struct {
	io.ReadCloser
	remaining  int
	limited    bool
	limitError error
}

func (b *modelResponseBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if b.limited {
		return 0, b.limitError
	}
	if len(p) > b.remaining+1 {
		p = p[:b.remaining+1]
	}
	n, err := b.ReadCloser.Read(p)
	if n > b.remaining {
		allowed := b.remaining
		b.remaining = 0
		b.limited = true
		return allowed, b.limitError
	}
	b.remaining -= n
	return n, err
}
