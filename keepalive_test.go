package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingServer counts the TCP connections it accepts.
func countingServer(t *testing.T, h http.Handler) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var conns atomic.Int32
	srv := httptest.NewUnstartedServer(h)
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			conns.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)

	return srv, &conns
}

func TestKeepWarmProbesUntilCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	done := make(chan struct{})

	go func() {
		keepWarm(ctx, 5*time.Millisecond, "t", func(context.Context) error {
			if calls.Add(1) == 3 {
				cancel()
			}
			return nil
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("keepWarm did not stop after its context was cancelled")
	}
	if calls.Load() < 3 {
		t.Fatalf("want at least 3 probes, got %d", calls.Load())
	}
}

func TestKeepWarmDisabled(t *testing.T) {
	done := make(chan struct{})
	go func() {
		keepWarm(context.Background(), 0, "t", func(context.Context) error {
			t.Error("probe ran with the keep-alive disabled")
			return nil
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("keepWarm with interval 0 should return immediately")
	}
}

// The probe has to travel the pooled connection real traffic uses, or it
// keeps the tunnel up while every forwarded request still opens its own.
func TestHttpProbeReusesOneConnection(t *testing.T) {
	var paths []string
	var mu sync.Mutex
	srv, conns := countingServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		// A 404 is as good as a 200: the target answered.
		http.Error(w, "not here", http.StatusNotFound)
	}))

	target, _ := url.Parse(srv.URL + "/base")
	client := &http.Client{Transport: newTargetTransport((&net.Dialer{}).DialContext)}
	p := httpProbe(client, target, "/api/")

	for range 3 {
		if err := p(context.Background()); err != nil {
			t.Fatalf("probe: %v", err)
		}
	}

	if n := conns.Load(); n != 1 {
		t.Fatalf("want 1 connection for 3 probes, got %d", n)
	}
	for _, p := range paths {
		if p != "/api/" {
			t.Fatalf("probe requested %q, want /api/ on the target's host", p)
		}
	}
}

func TestHttpProbeReportsUnreachableTarget(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	target, _ := url.Parse(srv.URL)
	srv.Close()

	client := &http.Client{Transport: newTargetTransport((&net.Dialer{}).DialContext)}
	if err := httpProbe(client, target, "/")(context.Background()); err == nil {
		t.Fatal("want an error for a target that is not listening")
	}
}

func TestTcpProbe(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	dial := (&net.Dialer{}).DialContext
	if err := tcpProbe(dial, ln.Addr().String())(context.Background()); err != nil {
		t.Fatalf("probe against a listening target: %v", err)
	}

	ln.Close()
	if err := tcpProbe(dial, ln.Addr().String())(context.Background()); err == nil {
		t.Fatal("want an error once the target stops listening")
	}
}

// Go's default keeps two idle connections per host, so the third of three
// simultaneous requests opened a new one every time. The pool now keeps them.
func TestTargetTransportKeepsConcurrentConnections(t *testing.T) {
	const parallel = 4

	var arrived sync.WaitGroup
	release := make(chan struct{})
	srv, conns := countingServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arrived.Done()
		<-release
		io.WriteString(w, "ok")
	}))

	client := &http.Client{Transport: newTargetTransport((&net.Dialer{}).DialContext)}

	round := func() {
		arrived.Add(parallel)
		release = make(chan struct{})
		var wg sync.WaitGroup
		for range parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				res, err := client.Get(srv.URL)
				if err != nil {
					t.Error(err)
					return
				}
				io.Copy(io.Discard, res.Body)
				res.Body.Close()
			}()
		}
		// Hold every request open until all have arrived, so they cannot
		// share connections within a round.
		arrived.Wait()
		close(release)
		wg.Wait()
	}

	round()
	round()

	if n := conns.Load(); n != parallel {
		t.Fatalf("want %d connections across two rounds of %d, got %d", parallel, parallel, n)
	}
}
