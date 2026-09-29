package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/half0wl/railtail/internal/logger"
)

const (
	// maxIdleConnsPerTarget is how many connections to the target are kept
	// open between requests. tsnet's own HTTPClient leaves Go's default of 2,
	// so a caller issuing three requests at once pays a fresh TCP handshake
	// over the tunnel for the third - one full round trip to the node.
	maxIdleConnsPerTarget = 16

	// targetIdleConnTimeout is how long an unused connection is kept. tsnet
	// leaves it unlimited, so the pool held connections until the target
	// dropped them; closing them first means a request is never written to a
	// connection the far end is in the middle of closing.
	targetIdleConnTimeout = 5 * time.Minute
)

// newTargetTransport replaces tsnet's HTTPClient transport, which sets
// nothing but the dialer.
func newTargetTransport(dial func(ctx context.Context, network, addr string) (net.Conn, error)) *http.Transport {
	return &http.Transport{
		DialContext:         dial,
		MaxIdleConnsPerHost: maxIdleConnsPerTarget,
		IdleConnTimeout:     targetIdleConnTimeout,
	}
}

func newHttpProxy(outboundClient *http.Client, target *url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.SetXForwarded()
		},
		Transport: outboundClient.Transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if !errors.Is(err, http.ErrAbortHandler) {
				logger.StderrWithSource.Error("failed to forward http request",
					logger.ErrAttr(err),
					slog.String("remote-addr", r.RemoteAddr),
					slog.String("target", target.Redacted()),
				)
			}

			w.WriteHeader(http.StatusBadGateway)
		},
	}
}
