package goba

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
)

func imageInputPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 120, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func imageInputHeader(t *testing.T, width, height uint32) []byte {
	t.Helper()
	data := imageInputPNG(t)
	binary.BigEndian.PutUint32(data[16:20], width)
	binary.BigEndian.PutUint32(data[20:24], height)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	return data
}

func TestAgentImageInlineAndDecodeBounds(t *testing.T) {
	data := imageInputPNG(t)
	got, err := loadAgentImage(context.Background(), http.DefaultClient, "base64://"+base64.StdEncoding.EncodeToString(data))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal(err)
	}
	if _, err = decodeAgentImage(got); err != nil {
		t.Fatal(err)
	}
	img, err := decodeAgentImage(data)
	if err != nil {
		t.Fatal(err)
	}
	var jpg bytes.Buffer
	if err = jpeg.Encode(&jpg, img, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = decodeAgentImage(jpg.Bytes()); err != nil {
		t.Fatal("JPEG lost", err)
	}
	for _, source := range []string{"", "base64://", "base64://!", "file:///synthetic.png", "C:/synthetic.png", "https://", "http://%zz"} {
		if _, err = loadAgentImage(context.Background(), http.DefaultClient, source); !errors.Is(err, errAgentImageInput) {
			t.Fatal(source, err)
		}
	}
	exact := bytes.Repeat([]byte{1}, agentImageBytes)
	if got, err = loadAgentImage(context.Background(), http.DefaultClient, "base64://"+base64.StdEncoding.EncodeToString(exact)); err != nil || len(got) != agentImageBytes {
		t.Fatal("exact inline bound rejected", err)
	}
	if _, err = loadAgentImage(context.Background(), http.DefaultClient, "base64://"+base64.StdEncoding.EncodeToString(append(exact, 1))); !errors.Is(err, errAgentImageLimit) {
		t.Fatal("oversize inline accepted", err)
	}
	for _, size := range [][2]uint32{{agentImageEdge + 1, 1}, {1, agentImageEdge + 1}, {4097, 4096}} {
		header := imageInputHeader(t, size[0], size[1])
		if _, _, err = image.DecodeConfig(bytes.NewReader(header)); err != nil {
			t.Fatal("invalid test header", err)
		}
		if _, err = decodeAgentImage(header); !errors.Is(err, errAgentImageLimit) {
			t.Fatal("oversize dimensions decoded", size, err)
		}
	}
	for _, bad := range [][]byte{nil, []byte("broken image"), data[:len(data)/2]} {
		if _, err = decodeAgentImage(bad); !errors.Is(err, errAgentImageInput) {
			t.Fatal("broken image accepted", err)
		}
	}
	if _, err = decodeAgentImage(append(exact, 1)); !errors.Is(err, errAgentImageLimit) {
		t.Fatal(err)
	}
}

func TestAgentImageHTTPDeadlineStatusAndDecodedBodyLimit(t *testing.T) {
	pngData := imageInputPNG(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.Write(pngData)
		case "/redirect":
			http.Redirect(w, r, "/ok", http.StatusFound)
		case "/status":
			http.Error(w, "secret fixture response", http.StatusBadGateway)
		case "/empty":
		case "/large-length":
			w.Header().Set("Content-Length", fmt.Sprint(agentImageBytes+1))
			w.WriteHeader(http.StatusOK)
		case "/exact":
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			io.CopyN(w, zeroImageReader{}, agentImageBytes)
		case "/large-stream":
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			io.CopyN(w, zeroImageReader{}, agentImageBytes+1)
		case "/gzip":
			w.Header().Set("Content-Encoding", "gzip")
			gz := gzip.NewWriter(w)
			io.CopyN(gz, zeroImageReader{}, agentImageBytes+1)
			gz.Close()
		case "/slow-header":
			<-r.Context().Done()
		case "/slow-body":
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}
	}))
	defer server.Close()
	for _, path := range []string{"/ok", "/redirect"} {
		got, err := loadAgentImage(context.Background(), server.Client(), server.URL+path)
		if err != nil || !bytes.Equal(got, pngData) {
			t.Fatal(path, err)
		}
	}
	if got, err := loadAgentImage(context.Background(), server.Client(), server.URL+"/exact"); err != nil || len(got) != agentImageBytes {
		t.Fatal("exact HTTP bound rejected", err)
	}
	for _, path := range []string{"/status", "/empty", "/large-length", "/large-stream", "/gzip"} {
		_, err := loadAgentImage(context.Background(), server.Client(), server.URL+path)
		if err == nil || strings.Contains(err.Error(), server.URL) || strings.Contains(err.Error(), "secret") {
			t.Fatal("invalid response accepted or exposed", path, err)
		}
		if strings.HasPrefix(path, "/large") || path == "/gzip" {
			if !errors.Is(err, errAgentImageLimit) {
				t.Fatal(path, err)
			}
		}
	}
	for _, path := range []string{"/slow-header", "/slow-body"} {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		started := time.Now()
		_, err := loadAgentImage(ctx, server.Client(), server.URL+path)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
			t.Fatal("download did not honor parent deadline", path, err)
		}
	}
}

type zeroImageReader struct{}

func (zeroImageReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

type imageInputTransport func(*http.Request) (*http.Response, error)

func (f imageInputTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type imageInputBody struct {
	io.Reader
	read   int
	closed bool
}

func (b *imageInputBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}
func (b *imageInputBody) Close() error { b.closed = true; return nil }

func TestAgentImageBodyClosesAndProductionDeadline(t *testing.T) {
	for _, tc := range []struct {
		status int
		length int64
		read   int
	}{{200, agentImageBytes + 1, 0}, {500, -1, 0}, {200, -1, agentImageBytes + 1}} {
		body := &imageInputBody{Reader: zeroImageReader{}}
		client := &http.Client{Transport: imageInputTransport(func(r *http.Request) (*http.Response, error) {
			deadline, ok := r.Context().Deadline()
			if !ok || time.Until(deadline) > agentImageFetchTimeout || time.Until(deadline) < 9*time.Second {
				t.Fatal("production deadline missing")
			}
			return &http.Response{StatusCode: tc.status, ContentLength: tc.length, Body: body, Header: make(http.Header)}, nil
		})}
		if _, err := loadAgentImage(context.Background(), client, "https://synthetic.invalid/image?key=secret"); err == nil || !body.closed || body.read != tc.read {
			t.Fatal("body not bounded/closed", err, body.read, body.closed)
		}
	}
}

func TestAgentVisionPreprocessingSkipsInvalidAndKeepsCache(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("bad vision request")
		}
		raw, _ := json.Marshal(body)
		if !bytes.Contains(raw, []byte("data:image/")) {
			t.Error("vision missing image")
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": "synthetic valid image description"}}}})
	}))
	defer server.Close()
	ag := NewAgent(&Config{}, 16, 8, time.Hour, "", nil, true, true)
	api := deepinfra.NewAPI(server.URL, "synthetic-key")
	api.SetHTTPClient(server.Client())
	ag.SetViewImageAPI(api, model.NewOpenAI("synthetic-vision", "", 0, 1, 128))
	data := imageInputPNG(t)
	raw, _ := json.Marshal([]any{map[string]any{"type": "image", "data": map[string]string{"file": "base64://!"}}, map[string]any{"type": "image", "data": map[string]string{"file": "base64://" + base64.StdEncoding.EncodeToString(imageInputHeader(t, 8193, 1))}}, map[string]any{"type": "image", "data": map[string]string{"file": "base64://" + base64.StdEncoding.EncodeToString(data)}}})
	ev := &Event{Message: raw}
	var item fmt.Stringer = ev
	ag.historyPre(&item)
	if calls.Load() != 1 {
		t.Fatal("invalid image requested vision or valid was skipped", calls.Load())
	}
	var segments []struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(ev.Message, &segments); err != nil {
		t.Fatal(err)
	}
	if len(segments) != 3 || segments[0].Data["__agent_desc__"] != "" || segments[1].Data["__agent_desc__"] != "" || segments[2].Data["__agent_desc__"] != "synthetic valid image description" {
		t.Fatal("description applied to wrong segment")
	}
	otherRaw, _ := json.Marshal([]any{map[string]any{"type": "image", "data": map[string]string{"file": "base64://" + base64.StdEncoding.EncodeToString(data)}}})
	other := &Event{Message: otherRaw}
	item = other
	ag.historyPre(&item)
	if calls.Load() != 1 || !bytes.Contains(other.Message, []byte("synthetic valid image description")) {
		t.Fatal("cache did not reuse valid description")
	}
	ag.ClearViewImageAPI()
	if ag.historyPre != nil || ag.CanViewImage() {
		t.Fatal("clear vision changed")
	}
}
