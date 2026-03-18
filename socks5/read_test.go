package socks5

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
)

func TestReadFullSuccess(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	data := []byte{0x05, 0x00, 0x00, 0x01}
	go func() {
		server.Write(data)
	}()

	buf := make([]byte, 4)
	err := ReadFull(client, buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, data) {
		t.Errorf("got %v, want %v", buf, data)
	}
}

func TestReadFullShortReads(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	data := []byte{0x05, 0x00, 0x00, 0x01, 0x02, 0x03}
	go func() {
		// Write in two chunks to simulate short reads
		server.Write(data[:3])
		server.Write(data[3:])
	}()

	buf := make([]byte, 6)
	err := ReadFull(client, buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, data) {
		t.Errorf("got %v, want %v", buf, data)
	}
}

func TestReadFullEOF(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	go func() {
		server.Write([]byte{0x05})
		server.Close()
	}()

	buf := make([]byte, 4)
	err := ReadFull(client, buf)
	if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("expected EOF or ErrClosedPipe, got: %v", err)
	}
}
