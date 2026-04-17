package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const DefaultLockTimeout = 3 * time.Hour

type Config struct {
	APIKey        string        `mapstructure:"api_key"`
	APIURL        string        `mapstructure:"api_url"`
	App           string        `mapstructure:"app"`
	Debug         bool          `mapstructure:"debug"`
	DiffThreads   int           `mapstructure:"diff_threads"`
	UploadRetries int           `mapstructure:"upload_retries"`
	LockTimeout   time.Duration `mapstructure:"lock_timeout"`

	// Output settings (from flags only, not config file)
	Format         string
	Quiet          bool
	Verbose        bool
	DryRun         bool
	Interactive    bool
	ProgressEvents bool

	// Config file paths that were loaded (for config show)
	LoadedFiles []ConfigFileStatus
}

type ConfigFileStatus struct {
	Path   string
	Status string // "loaded", "not_found", "error"
	Error  string
}

type ConfigValue struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

func projectConfigPath() string {
	return ".patchkit.yml"
}

func userConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "patchkit", "config.yml")
}

func SetDefaults() {
	viper.SetDefault("api_url", "https://api.patchkit.net")
	viper.SetDefault("debug", false)
	viper.SetDefault("diff_threads", 4)
	viper.SetDefault("upload_retries", 5)
	viper.SetDefault("lock_timeout", DefaultLockTimeout)
}

func BindEnvVars() {
	viper.SetEnvPrefix("")
	viper.BindEnv("api_key", "PATCHKIT_API_KEY")
	viper.BindEnv("app", "PATCHKIT_APP")
	viper.BindEnv("api_url", "PATCHKIT_API_URL")
	viper.BindEnv("debug", "PATCHKIT_DEBUG")
}

func BindFlags(cmd *cobra.Command) {
	viper.BindPFlag("api_key", cmd.Root().PersistentFlags().Lookup("api-key"))
	viper.BindPFlag("api_url", cmd.Root().PersistentFlags().Lookup("api-url"))
	viper.BindPFlag("debug", cmd.Root().PersistentFlags().Lookup("debug"))
}

func Load() (*Config, error) {
	SetDefaults()
	BindEnvVars()

	cfg := &Config{}

	// Try project-local config
	projectPath := projectConfigPath()
	projectStatus := loadConfigFile(projectPath)
	cfg.LoadedFiles = append(cfg.LoadedFiles, projectStatus)

	// Try user-level config
	userPath := userConfigPath()
	if userPath != "" {
		userStatus := loadConfigFile(userPath)
		cfg.LoadedFiles = append(cfg.LoadedFiles, userStatus)
	}

	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Handle lock_timeout as string duration
	if lt := viper.GetString("lock_timeout"); lt != "" {
		if d, err := time.ParseDuration(lt); err == nil {
			cfg.LockTimeout = d
		}
	}

	return cfg, nil
}

func loadConfigFile(path string) ConfigFileStatus {
	status := ConfigFileStatus{Path: path}

	absPath, err := filepath.Abs(path)
	if err != nil {
		status.Status = "error"
		status.Error = err.Error()
		return status
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		status.Status = "not_found"
		return status
	}

	viper.SetConfigFile(absPath)
	viper.SetConfigType("yaml")
	if err := viper.MergeInConfig(); err != nil {
		status.Status = "error"
		status.Error = err.Error()
		return status
	}

	status.Status = "loaded"
	return status
}

func (c *Config) Validate() error {
	if c.APIURL == "" {
		return fmt.Errorf("API URL is required")
	}
	return nil
}

func (c *Config) RequireAPIKey() error {
	if c.APIKey == "" {
		return fmt.Errorf("API key is required. Set PATCHKIT_API_KEY or use --api-key")
	}
	return nil
}

func (c *Config) RequireApp() error {
	if c.App == "" {
		return fmt.Errorf("application secret is required. Set PATCHKIT_APP or use --app")
	}
	return nil
}

func (c *Config) MaskedAPIKey() string {
	if c.APIKey == "" {
		return ""
	}
	if len(c.APIKey) <= 8 {
		return "***"
	}
	return c.APIKey[:4] + "..." + c.APIKey[len(c.APIKey)-4:]
}

func (c *Config) GetResolvedValues() map[string]ConfigValue {
	values := make(map[string]ConfigValue)

	values["api_key"] = ConfigValue{
		Value:  c.MaskedAPIKey(),
		Source: resolveSource("api_key"),
	}
	values["api_url"] = ConfigValue{
		Value:  c.APIURL,
		Source: resolveSource("api_url"),
	}
	values["app"] = ConfigValue{
		Value:  c.App,
		Source: resolveSource("app"),
	}
	values["debug"] = ConfigValue{
		Value:  fmt.Sprintf("%v", c.Debug),
		Source: resolveSource("debug"),
	}
	values["diff_threads"] = ConfigValue{
		Value:  fmt.Sprintf("%d", c.DiffThreads),
		Source: resolveSource("diff_threads"),
	}
	values["upload_retries"] = ConfigValue{
		Value:  fmt.Sprintf("%d", c.UploadRetries),
		Source: resolveSource("upload_retries"),
	}
	values["lock_timeout"] = ConfigValue{
		Value:  c.LockTimeout.String(),
		Source: resolveSource("lock_timeout"),
	}

	return values
}

func resolveSource(key string) string {
	// Check if flag was explicitly set
	if viper.IsSet(key) {
		// Check env var
		envKeys := map[string]string{
			"api_key": "PATCHKIT_API_KEY",
			"app":     "PATCHKIT_APP",
			"api_url": "PATCHKIT_API_URL",
			"debug":   "PATCHKIT_DEBUG",
		}
		if envName, ok := envKeys[key]; ok {
			if os.Getenv(envName) != "" {
				return "env:" + envName
			}
		}

		// Check config files
		for _, f := range []string{projectConfigPath(), userConfigPath()} {
			if f != "" {
				if _, err := os.Stat(f); err == nil {
					return "file:" + f
				}
			}
		}
	}

	return "default"
}

func CheckGitignoreWarning() string {
	projectPath := projectConfigPath()
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		return ""
	}

	// Check if file contains secrets
	data, err := os.ReadFile(projectPath)
	if err != nil {
		return ""
	}
	content := string(data)
	hasSecrets := strings.Contains(content, "api_key") || strings.Contains(content, "app:")

	if !hasSecrets {
		return ""
	}

	// Check .gitignore
	gitignore, err := os.ReadFile(".gitignore")
	if err != nil {
		return fmt.Sprintf("Warning: %s contains secrets (api_key, app) but .gitignore not found", projectPath)
	}

	if !strings.Contains(string(gitignore), projectPath) {
		return fmt.Sprintf("Warning: %s contains secrets (api_key, app) but is not in .gitignore", projectPath)
	}

	return ""
}

// ReadChangelog reads changelog content. If value starts with @, reads from file.
func ReadChangelog(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "@") {
		filePath := value[1:]
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to read changelog file %s: %w", filePath, err)
		}
		return string(data), nil
	}
	return value, nil
}
