//go:build ignore

// Download and validate the author's complete image set before installing it.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"
)

const source = "https://codeload.github.com/fumiama/senso-ji-omikuji/zip/refs/heads/main"

func main() {
	install := flag.Bool("install", false, "install validated images into data/Omikuji")
	flag.Parse()
	root, err := os.Getwd()
	must(err)
	parent := filepath.Join(root, ".local", "omikuji-assets")
	must(os.MkdirAll(parent, 0700))
	out, err := os.MkdirTemp(parent, "run-")
	must(err)
	fmt.Println("ARTIFACTS=" + out)
	client := &http.Client{Timeout: 90 * time.Second}
	var archive []byte
	for attempt := 1; attempt <= 3; attempt++ {
		archive, err = download(client)
		if err == nil {
			break
		}
		fmt.Printf("download attempt %d failed: %v\n", attempt, err)
	}
	must(err)
	must(os.WriteFile(filepath.Join(out, "source.zip"), archive, 0600))
	z, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	must(err)
	wanted := map[string]bool{"LICENSE": true}
	for n := 1; n <= 100; n++ {
		for side := 0; side < 2; side++ {
			wanted[fmt.Sprintf("%d_%d.jpg", n, side)] = true
		}
	}
	files := map[string][]byte{}
	for _, f := range z.File {
		name := path.Base(f.Name)
		if !wanted[name] || f.FileInfo().IsDir() {
			continue
		}
		if _, exists := files[name]; exists {
			panic("duplicate archive member: " + name)
		}
		r, e := f.Open()
		must(e)
		data, e := io.ReadAll(io.LimitReader(r, 4*1024*1024+1))
		r.Close()
		must(e)
		if len(data) == 0 || len(data) > 4*1024*1024 {
			panic("invalid member size: " + name)
		}
		if name != "LICENSE" {
			img, e := jpeg.Decode(bytes.NewReader(data))
			must(e)
			if img.Bounds().Empty() {
				panic("empty JPEG")
			}
		}
		files[name] = data
	}
	if len(files) != len(wanted) {
		panic(fmt.Sprintf("expected 200 images and license; got %d files", len(files)))
	}
	hashes := map[string]string{}
	destination := filepath.Join(root, "data", "Omikuji")
	if *install {
		must(os.MkdirAll(destination, 0755))
	}
	installed := 0
	for name, data := range files {
		h := sha256.Sum256(data)
		hashes[name] = hex.EncodeToString(h[:])
		must(os.WriteFile(filepath.Join(out, name), data, 0600))
		if !*install {
			continue
		}
		target := filepath.Join(destination, name)
		old, e := os.ReadFile(target)
		if e == nil {
			if bytes.Equal(old, data) {
				continue
			}
			must(os.MkdirAll(filepath.Join(out, "previous"), 0700))
			must(os.WriteFile(filepath.Join(out, "previous", name), old, 0600))
		} else if !os.IsNotExist(e) {
			must(e)
		}
		f, e := os.CreateTemp(destination, ".omikuji-*")
		must(e)
		tmp := f.Name()
		_, e = f.Write(data)
		closeErr := f.Close()
		must(e)
		must(closeErr)
		// Rename only a fully validated complete file into the cache.
		must(os.Rename(tmp, target))
		installed++
		actual, e := os.ReadFile(target)
		must(e)
		if !bytes.Equal(data, actual) {
			panic("installed file mismatch")
		}
	}
	h := sha256.Sum256(archive)
	report := map[string]any{"passed": true, "source": source, "archiveSHA256": hex.EncodeToString(h[:]), "validatedJPEGs": 200, "installed": *install, "writtenFiles": installed, "fileSHA256": hashes, "completedAt": time.Now().Format(time.RFC3339)}
	b, err := json.MarshalIndent(report, "", "  ")
	must(err)
	must(os.WriteFile(filepath.Join(out, "result.json"), b, 0600))
	fmt.Printf("validatedJPEGs=200 installed=%v writtenFiles=%d\n", *install, installed)
}

func download(c *http.Client) ([]byte, error) {
	r, err := c.Get(source)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", r.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 64*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 64*1024*1024 {
		return nil, fmt.Errorf("archive too large")
	}
	return b, nil
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
