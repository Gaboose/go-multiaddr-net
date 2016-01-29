package manet

import (
	"fmt"
	"io"
	"net"
	"testing"

	ma "github.com/jbenet/go-multiaddr"
)

//TODO:
// delete closed listeners from relisten
// rethink listener, conn, close paths (wcon.Close() doesn't close netcon?)
// clen up after errors from context mutators (remove from relisten? close all listeners? one?), same for conns

func TestDial(t *testing.T) {

	ln, err := net.Listen("tcp", "127.0.0.1:4324")
	if err != nil {
		t.Error(err)
	}

	go func() {
		c, err := ln.Accept()
		if err != nil {
			t.Error(err)
		}
		defer c.Close()
		echoOnce(c)
	}()

	m, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/4324")
	if err != nil {
		t.Error(err)
	}

	c, err := Dial(m)
	if err != nil {
		t.Error(err)
		t.Fatalf("couldn't dial %s", m.String())
	}
	defer c.Close()

	assertEcho(t, c)
}

func echoOnce(rw io.ReadWriter) error {
	buf := make([]byte, 256)
	n, err := rw.Read(buf)
	if err != nil {
		return err
	}
	_, err = rw.Write(buf[:n])
	return err
}

func assertEcho(t *testing.T, rw io.ReadWriter) {
	str := "test string"
	_, err := fmt.Fprint(rw, str)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 256)
	n, err := rw.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	got := string(buf[:n])

	if got != str {
		t.Fatalf("expected \"%s\", got \"%s\"", str, got)
	}
}
