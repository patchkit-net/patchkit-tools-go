package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestDefaultLockTimeout(t *testing.T) {
	if DefaultLockTimeout != 3*time.Hour {
		t.Errorf("DefaultLockTimeout = %v, want %v", DefaultLockTimeout, 3*time.Hour)
	}
}

func TestSetDefaults_LockTimeout(t *testing.T) {
	viper.Reset()
	SetDefaults()
	got := viper.GetDuration("lock_timeout")
	if got != 3*time.Hour {
		t.Errorf("viper default lock_timeout = %v, want %v", got, 3*time.Hour)
	}
}

func TestReadChangelog_inline(t *testing.T) {
	text, err := ReadChangelog("Fixed login bug")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Fixed login bug" {
		t.Errorf("got %q, want %q", text, "Fixed login bug")
	}
}

func TestReadChangelog_empty(t *testing.T) {
	text, err := ReadChangelog("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "" {
		t.Errorf("got %q, want empty", text)
	}
}

func TestReadChangelog_fromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CHANGELOG.md")
	content := "## v1.0.0\n- Fixed bug\n- Added feature"
	os.WriteFile(path, []byte(content), 0644)

	text, err := ReadChangelog("@" + path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != content {
		t.Errorf("got %q, want %q", text, content)
	}
}

func TestReadChangelog_fromFile_notFound(t *testing.T) {
	_, err := ReadChangelog("@/nonexistent/file.md")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestConfig_MaskedAPIKey(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"", ""},
		{"short", "***"},
		{"abcd1234efgh", "abcd...efgh"},
		{"sk_live_abcdefghijklmnop", "sk_l...mnop"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			c := &Config{APIKey: tt.key}
			if got := c.MaskedAPIKey(); got != tt.want {
				t.Errorf("MaskedAPIKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfig_RequireAPIKey(t *testing.T) {
	c := &Config{}
	if err := c.RequireAPIKey(); err == nil {
		t.Error("expected error for empty API key")
	}

	c.APIKey = "somekey"
	if err := c.RequireAPIKey(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConfig_RequireApp(t *testing.T) {
	c := &Config{}
	if err := c.RequireApp(); err == nil {
		t.Error("expected error for empty app")
	}

	c.App = "abc123"
	if err := c.RequireApp(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConfig_Validate(t *testing.T) {
	c := &Config{}
	if err := c.Validate(); err == nil {
		t.Error("expected error for empty API URL")
	}

	c.APIURL = "https://api.patchkit.net"
	if err := c.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
