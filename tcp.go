package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"golang.org/x/sync/errgroup"
	"tailscale.com/tsnet"
)

const dialTimeout = 30 * time.Second

// closeWriter is implemented by *net.TCPConn and by tsnet's gonet.TCPConn.
type closeWriter interface {
	CloseWrite() error
}

func fwdTCP(lstConn net.Conn, ts *tsnet.Server, targetAddr string) error {
	defer lstConn.Close()

	dialCtx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	tsConn, err := ts.Dial(dialCtx, "tcp", targetAddr)
	if err != nil {
		return fmt.Errorf("failed to dial tailscale node: %w", err)
	}

	defer tsConn.Close()

	var g errgroup.Group

	g.Go(func() error {
		return pipe(tsConn, lstConn, "to tailscale node")
	})

	g.Go(func() error {
		return pipe(lstConn, tsConn, "from tailscale node")
	})

	if err := g.Wait(); err != nil {
		return fmt.Errorf("connection error: %w", err)
	}

	return nil
}

// pipe copies src into dst, then passes the EOF on to dst. Without that, a
// side that hangs up leaves the other side's copy blocked forever, and the
// connection pair (and its goroutines) is never released.
func pipe(dst, src net.Conn, direction string) error {
	_, err := io.Copy(dst, src)

	if cw, ok := dst.(closeWriter); ok {
		if cwErr := cw.CloseWrite(); cwErr != nil {
			dst.Close()
		}
	} else {
		dst.Close()
	}

	if err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("failed to copy data %s: %w", direction, err)
	}

	return nil
}
