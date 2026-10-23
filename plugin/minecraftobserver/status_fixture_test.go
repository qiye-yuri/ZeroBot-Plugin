package minecraftobserver

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

// A local Java status endpoint keeps regression tests independent of public servers.
func mockStatusServer(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				r := bufio.NewReader(conn)
				read := func() ([]byte, error) {
					n, e := binary.ReadUvarint(r)
					if e != nil {
						return nil, e
					}
					if n > 4096 {
						return nil, io.ErrShortBuffer
					}
					b := make([]byte, int(n))
					_, e = io.ReadFull(r, b)
					return b, e
				}
				write := func(b []byte) error {
					var prefix [10]byte
					n := binary.PutUvarint(prefix[:], uint64(len(b)))
					_, e := conn.Write(append(prefix[:n], b...))
					return e
				}
				if _, err := read(); err != nil {
					return
				} // handshake
				if _, err := read(); err != nil {
					return
				} // status request
				status := []byte(`{"version":{"name":"1.20.1","protocol":763},"players":{"max":20,"online":2},"description":{"text":"本地测试服务器"}}`)
				var size [10]byte
				n := binary.PutUvarint(size[:], uint64(len(status)))
				packet := append([]byte{0}, size[:n]...)
				packet = append(packet, status...)
				if write(packet) != nil {
					return
				}
				ping, err := read()
				if err == nil {
					_ = write(ping)
				}
			}()
		}
	}()
	return l.Addr().String()
}

func closedStatusAddress(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}
