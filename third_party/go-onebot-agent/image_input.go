package goba

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	agentImageBytes        = 8 << 20
	agentImagePixels       = 16 << 20
	agentImageEdge         = 8192
	agentImageFetchTimeout = 10 * time.Second
)

// Errors deliberately contain no URL, response body or embedded image content.
var (
	errAgentImageInput = errors.New("unsupported or invalid Agent image input")
	errAgentImageLimit = errors.New("Agent image exceeds input limits")
	errAgentImageFetch = errors.New("Agent image download failed")
)

// The deadline includes redirects and reading the body. It does not describe
// the separate vision model request or the duration of all images in a task.
func loadAgentImage(parent context.Context, client *http.Client, source string) ([]byte, error) {
	if strings.HasPrefix(source, "base64://") {
		encoded := source[len("base64://"):]
		if len(encoded) > base64.StdEncoding.EncodedLen(agentImageBytes) {
			return nil, errAgentImageLimit
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(data) == 0 {
			return nil, errAgentImageInput
		}
		if len(data) > agentImageBytes {
			return nil, errAgentImageLimit
		}
		return data, nil
	}
	u, err := url.Parse(source)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errAgentImageInput
	}
	ctx, cancel := context.WithTimeout(parent, agentImageFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, errAgentImageInput
	}
	rsp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errAgentImageFetch
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, errAgentImageFetch
	}
	if rsp.ContentLength > agentImageBytes {
		return nil, errAgentImageLimit
	}
	data, err := io.ReadAll(io.LimitReader(rsp.Body, agentImageBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errAgentImageFetch
	}
	if len(data) > agentImageBytes {
		return nil, errAgentImageLimit
	}
	if len(data) == 0 {
		return nil, errAgentImageInput
	}
	return data, nil
}

// Reject excessive dimensions before allocating the full decoded image.
func decodeAgentImage(data []byte) (image.Image, error) {
	if len(data) == 0 {
		return nil, errAgentImageInput
	}
	if len(data) > agentImageBytes {
		return nil, errAgentImageLimit
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, errAgentImageInput
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > agentImageEdge || cfg.Height > agentImageEdge || cfg.Width > agentImagePixels/cfg.Height {
		return nil, errAgentImageLimit
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errAgentImageInput
	}
	return img, nil
}
