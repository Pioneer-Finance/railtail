package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/half0wl/railtail/internal/config"
	"github.com/half0wl/railtail/internal/logger"
)

// maxProbeTimeout bounds one keep-alive probe. A probe that outlives its
// interval would only queue behind itself.
const maxProbeTimeout = 10 * time.Second

// probe sends one request's worth of traffic to the target. Any answer the
// target gives counts: the point is that packets crossed the tunnel, not what
// the target thought of them.
type probe func(ctx context.Context) error

// keepWarm runs p every interval until ctx is done.
//
// A Tailscale path that carries nothing for a couple of minutes has to be
// re-established before the next packet gets through - measured against a
// DERP-relayed node, the first request after 150s of silence took ~620ms
// against ~200ms warm. A caller with bursty traffic paid that on almost every
// burst. Sending a little traffic on a timer keeps the session up, and in HTTP
// mode keeps a pooled connection open as well.
//
// Failures are logged when they start and when they stop, not on every tick:
// a target that is down for an hour should not write sixty lines saying so.
func keepWarm(ctx context.Context, interval time.Duration, target string, p probe) {
	if interval <= 0 {
		return
	}

	timeout := min(interval, maxProbeTimeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	failing := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		probeCtx, cancel := context.WithTimeout(ctx, timeout)
		err := p(probeCtx)
		cancel()

		switch {
		case err != nil && !failing:
			failing = true
			logger.StderrWithSource.Warn("keep-alive probe failed; the target may be unreachable",
				logger.ErrAttr(err),
				slog.String("target", target),
			)
		case err == nil && failing:
			failing = false
			logger.Stdout.Info("keep-alive probe succeeded again", slog.String("target", target))
		}
	}
}

func logKeepalive(cfg *config.Config) {
	if cfg.KeepaliveInterval <= 0 {
		logger.Stdout.Info("keep-alive disabled (KEEPALIVE_INTERVAL=0)")
		return
	}

	attrs := []any{slog.String("interval", cfg.KeepaliveInterval.String())}
	if cfg.TargetURL != nil {
		attrs = append(attrs, slog.String("path", cfg.KeepalivePath))
	}
	logger.Stdout.Info("keep-alive enabled", attrs...)
}

// httpProbe requests path on target's host through client, so the probe
// travels the same pooled connections as forwarded traffic.
func httpProbe(client *http.Client, target *url.URL, path string) probe {
	u := url.URL{Scheme: target.Scheme, User: target.User, Host: target.Host, Path: path}

	return func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "railtail-keepalive")

		res, err := client.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		// Drain a bounded amount so the connection goes back to the pool;
		// an unread body means the transport closes it instead.
		_, err = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
		return err
	}
}

// tcpProbe opens and closes a connection to addr. TCP mode has no pool to
// keep, but the handshake alone keeps the Tailscale path to the node up.
func tcpProbe(dial func(ctx context.Context, network, addr string) (net.Conn, error), addr string) probe {
	return func(ctx context.Context) error {
		conn, err := dial(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("dial %s: %w", addr, err)
		}
		return conn.Close()
	}
}
