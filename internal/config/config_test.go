package config

import (
	"flag"
	"os"
	"strings"
	"testing"
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
