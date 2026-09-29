package config

import (
	"flag"
	"os"
	"strings"
	"testing"
	"time"
)

func load(t *testing.T, env map[string]string) (*Config, []error) {
	t.Helper()
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	os.Args = os.Args[:1]
	for k, v := range map[string]string{"TS_HOSTNAME": "h", "LISTEN_PORT": "1", "TS_AUTHKEY": "k"} {
		t.Setenv(k, v)
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	return LoadConfig()
}

func TestTLSVerificationIsOnByDefault(t *testing.T) {
	cfg, errs := load(t, map[string]string{"TARGET_ADDR": "https://db:443"})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if cfg.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify should default to false")
	}
}

func TestInsecureSkipVerifyOptIn(t *testing.T) {
	cfg, errs := load(t, map[string]string{"TARGET_ADDR": "https://db:443", "INSECURE_SKIP_VERIFY": "true"})
	if len(errs) > 0 || !cfg.InsecureSkipVerify {
		t.Fatalf("want opt-in honoured, got %v %v", cfg, errs)
	}
	if _, errs := load(t, map[string]string{"TARGET_ADDR": "https://db:443", "INSECURE_SKIP_VERIFY": "yes please"}); len(errs) == 0 {
		t.Fatal("want an error for a non-boolean value")
	}
}

func TestTargetCredentialsAreNotLogged(t *testing.T) {
	cfg, errs := load(t, map[string]string{"TARGET_ADDR": "https://user:s3cret@db:443"})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if strings.Contains(cfg.RedactedTargetAddr(), "s3cret") {
		t.Fatalf("password leaked: %s", cfg.RedactedTargetAddr())
	}
	_, errs = load(t, map[string]string{"TARGET_ADDR": "https://user:s3cret@db%zz:443"})
	for _, e := range errs {
		if strings.Contains(e.Error(), "s3cret") {
			t.Fatalf("password leaked in error: %v", e)
		}
	}
}

func TestKeepaliveDefaults(t *testing.T) {
	cfg, errs := load(t, map[string]string{"TARGET_ADDR": "http://db:8086"})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if cfg.KeepaliveInterval != 60*time.Second || cfg.KeepalivePath != "/" {
		t.Fatalf("got interval %s path %q, want 60s and /", cfg.KeepaliveInterval, cfg.KeepalivePath)
	}
}

func TestKeepaliveSettings(t *testing.T) {
	cfg, errs := load(t, map[string]string{"TARGET_ADDR": "db:5432", "KEEPALIVE_INTERVAL": "0"})
	if len(errs) > 0 || cfg.KeepaliveInterval != 0 {
		t.Fatalf("0 should disable the keep-alive, got %s %v", cfg.KeepaliveInterval, errs)
	}

	cfg, errs = load(t, map[string]string{"TARGET_ADDR": "http://db:8086", "KEEPALIVE_INTERVAL": "45s", "KEEPALIVE_PATH": "/api/"})
	if len(errs) > 0 || cfg.KeepaliveInterval != 45*time.Second || cfg.KeepalivePath != "/api/" {
		t.Fatalf("got %s %q %v", cfg.KeepaliveInterval, cfg.KeepalivePath, errs)
	}

	for name, env := range map[string]map[string]string{
		"not a duration": {"KEEPALIVE_INTERVAL": "soon"},
		"negative":       {"KEEPALIVE_INTERVAL": "-5s"},
		"relative path":  {"KEEPALIVE_PATH": "api/"},
	} {
		env["TARGET_ADDR"] = "http://db:8086"
		if _, errs := load(t, env); len(errs) == 0 {
			t.Errorf("%s: want a configuration error", name)
		}
	}
}
