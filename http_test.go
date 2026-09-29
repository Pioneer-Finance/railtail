package main

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestHttpProxyForwardsPathAndHost(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, r.Host+" "+r.URL.RequestURI())
	}))
	defer backend.Close()

	target, _ := url.Parse(backend.URL + "/base")
	front := httptest.NewServer(newHttpProxy(&http.Client{Transport: http.DefaultTransport}, target))
	defer front.Close()

	res, err := http.Get(front.URL + "/x?y=1")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	if want := target.Host + " /base/x?y=1"; string(body) != want {
		t.Fatalf("got %q want %q", body, want)
	}
}

// Certificates are verified unless the operator opts out.
func TestHttpProxyRejectsUntrustedCertificate(t *testing.T) {
	backend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer backend.Close()

	target, _ := url.Parse(backend.URL)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	front := httptest.NewServer(newHttpProxy(&http.Client{Transport: transport}, target))
	defer front.Close()

	res, err := http.Get(front.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("got %d, want 502 for an untrusted certificate", res.StatusCode)
	}

	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	res, err = http.Get(front.URL)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("with opt-out got %v %v, want 200", res, err)
	}
}
