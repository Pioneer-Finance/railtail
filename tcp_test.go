package main

import (
	"io"
	"net"
	"testing"
	"time"
)

func tcpPair(t *testing.T) (client, server net.Conn) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		accepted <- c
	}()

	client, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	return client, <-accepted
}

// A caller that hangs up must release the upstream connection even though the
// upstream never closes on its own.
func TestPipeReleasesBothSidesWhenOneHangsUp(t *testing.T) {
	caller, lst := tcpPair(t)
	upstreamPeer, up := tcpPair(t)
	defer upstreamPeer.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		go pipe(up, lst, "to")
		pipe(lst, up, "from")
	}()

	// Half-close: the request still gets its response.
	caller.Write([]byte("ping"))
	caller.(*net.TCPConn).CloseWrite()

	buf, err := io.ReadAll(upstreamPeer)
	if err != nil || string(buf) != "ping" {
		t.Fatalf("upstream got %q, %v; want ping then EOF", buf, err)
	}

	upstreamPeer.Write([]byte("pong"))
	upstreamPeer.(*net.TCPConn).CloseWrite()

	caller.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf, err = io.ReadAll(caller)
	if err != nil || string(buf) != "pong" {
		t.Fatalf("caller got %q, %v; want pong then EOF", buf, err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pipe did not finish after both sides closed")
	}
}
