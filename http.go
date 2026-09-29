package main

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/half0wl/railtail/internal/logger"
)

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
