package cli

import (
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/patchkit-net/patchkit-tools-go/internal/config"
)

func TestResolveLockTimeout_FlagUnchanged_UsesConfig(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("lock-timeout", config.DefaultLockTimeout.String(), "")
	cfg := &config.Config{LockTimeout: 5 * time.Hour}

	got, err := resolveLockTimeout(cmd, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 5*time.Hour {
		t.Errorf("got %v, want %v (from config)", got, 5*time.Hour)
	}
}

func TestResolveLockTimeout_FlagChanged_UsesFlag(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("lock-timeout", config.DefaultLockTimeout.String(), "")
	if err := cmd.Flags().Set("lock-timeout", "1h"); err != nil {
		t.Fatalf("flag set failed: %v", err)
	}
	cfg := &config.Config{LockTimeout: 5 * time.Hour}

	got, err := resolveLockTimeout(cmd, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != time.Hour {
		t.Errorf("got %v, want %v (from flag)", got, time.Hour)
	}
}

func TestResolveLockTimeout_InvalidFlag_ReturnsError(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("lock-timeout", config.DefaultLockTimeout.String(), "")
	if err := cmd.Flags().Set("lock-timeout", "not-a-duration"); err != nil {
		t.Fatalf("flag set failed: %v", err)
	}
	cfg := &config.Config{LockTimeout: 5 * time.Hour}

	if _, err := resolveLockTimeout(cmd, cfg); err == nil {
		t.Error("expected error for invalid duration, got nil")
	}
}

func TestVersionPushCmd_LockTimeoutDefault(t *testing.T) {
	cmd := newVersionPushCmd()
	flag := cmd.Flags().Lookup("lock-timeout")
	if flag == nil {
		t.Fatal("version push: --lock-timeout flag not found")
	}
	want := config.DefaultLockTimeout.String()
	if flag.DefValue != want {
		t.Errorf("version push --lock-timeout default = %q, want %q", flag.DefValue, want)
	}
}

func TestChannelPushCmd_LockTimeoutDefault(t *testing.T) {
	cmd := newChannelPushCmd()
	flag := cmd.Flags().Lookup("lock-timeout")
	if flag == nil {
		t.Fatal("channel push: --lock-timeout flag not found")
	}
	want := config.DefaultLockTimeout.String()
	if flag.DefValue != want {
		t.Errorf("channel push --lock-timeout default = %q, want %q", flag.DefValue, want)
	}
}

func TestVersionImportCmd_LockTimeoutFlag(t *testing.T) {
	cmd := newVersionImportCmd()
	flag := cmd.Flags().Lookup("lock-timeout")
	if flag == nil {
		t.Fatal("version import: --lock-timeout flag not found")
	}
	want := config.DefaultLockTimeout.String()
	if flag.DefValue != want {
		t.Errorf("version import --lock-timeout default = %q, want %q", flag.DefValue, want)
	}
}
