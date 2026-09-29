package config

import (
	"errors"
	stderrors "errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/half0wl/railtail/internal/config/parser"
)

type ForwardTrafficType string

const (
	ForwardTrafficTypeTCP   ForwardTrafficType = "tcp"
	ForwardTrafficTypeHTTP  ForwardTrafficType = "http"
	ForwardTrafficTypeHTTPS ForwardTrafficType = "https"
)

var (
	ErrTargetAddrInvalid = errors.New("target-addr is invalid")
)

type Config struct {
	TSHostname     string `flag:"ts-hostname" env:"TS_HOSTNAME" usage:"hostname to use for tailscale"`
	ListenPort     string `flag:"listen-port" env:"LISTEN_PORT" usage:"port to listen on"`
	TargetAddr     string `flag:"target-addr" env:"TARGET_ADDR" usage:"address:port of a tailscale node to send traffic to"`
	TSLoginServer  string `flag:"ts-login-server" env:"TS_LOGIN_SERVER" default:"" usage:"base url of the control server, If you are using Headscale for your control server, use your Headscale instance's URL"`
	TSStateDirPath string `flag:"ts-state-dir" env:"TS_STATEDIR_PATH" default:"/tmp/railtail" usage:"tailscale state dir"`
	TSAuthKey      string `env:"TS_AUTHKEY,TS_AUTH_KEY" usage:"tailscale auth key"`

	InsecureSkipVerifyRaw string `flag:"insecure-skip-verify" env:"INSECURE_SKIP_VERIFY" default:"false" usage:"skip TLS certificate verification of an https target (not recommended)"`

	ForwardTrafficType ForwardTrafficType
	InsecureSkipVerify bool
	TargetURL          *url.URL
}

// RedactedTargetAddr is TargetAddr with any userinfo password masked, for logging.
func (c *Config) RedactedTargetAddr() string {
	if c.TargetURL != nil {
		return c.TargetURL.Redacted()
	}

	return c.TargetAddr
}

func init() {
	// add help flag purely for the usage message
	flag.Bool("help", false, "Show help message")

	// Only parse and print usage if -help is present in arguments
	if checkForFlag("help") {
		// Create temporary config just to register all flags for usage message
		cfg := &Config{}

		parser.ParseFlags(cfg)

		flag.Usage()
		os.Exit(0)
	}
}

func LoadConfig() (*Config, []error) {
	cfg := &Config{}

	errors := parser.ParseConfig(cfg)

	if cfg.InsecureSkipVerifyRaw != "" {
		skip, err := strconv.ParseBool(cfg.InsecureSkipVerifyRaw)
		if err != nil {
			errors = append(errors, fmt.Errorf("insecure-skip-verify must be true or false: %w", err))
		}

		cfg.InsecureSkipVerify = skip
	}

	// Validate target-addr if it's set to either be a valid URL with a port or a valid address:port
	if cfg.TargetAddr != "" {
		protocol := strings.SplitN(cfg.TargetAddr, "://", 2)[0]

		switch protocol {
		case "https", "http":
			cfg.ForwardTrafficType = ForwardTrafficType(protocol)

			u, err := url.Parse(cfg.TargetAddr)
			if err != nil {
				// url.Error embeds the raw URL, which may carry credentials; report only the cause.
				var urlErr *url.Error
				if stderrors.As(err, &urlErr) {
					err = urlErr.Err
				}

				errors = append(errors, fmt.Errorf("%w: %w", ErrTargetAddrInvalid, err))
			}

			// Check if the URL has a port only if the URL is valid
			if err == nil && u.Port() == "" {
				errors = append(errors, fmt.Errorf("%w: address %s: missing port in address", ErrTargetAddrInvalid, u.Redacted()))
			}

			cfg.TargetURL = u
		default:
			cfg.ForwardTrafficType = ForwardTrafficTypeTCP

			_, _, err := net.SplitHostPort(cfg.TargetAddr)
			if err != nil {
				errors = append(errors, fmt.Errorf("%w: %w", ErrTargetAddrInvalid, err))
			}
		}
	}

	if len(errors) > 0 {
		return nil, errors
	}

	return cfg, nil
}
