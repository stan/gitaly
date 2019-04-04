package config

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/kelseyhightower/envconfig"
	log "github.com/sirupsen/logrus"
)

const (
	// EnvPidFile is the name of the environment variable containing the pid file path
	EnvPidFile = "GITALY_PID_FILE"
	// EnvUpgradesEnabled is an environment variable that when defined gitaly must enable graceful upgrades on SIGHUP
	EnvUpgradesEnabled = "GITALY_UPGRADES_ENABLED"
)

var (
	// Config stores the global configuration
	Config config
)

type config struct {
	SocketPath                 string        `toml:"socket_path" split_words:"true"`
	ListenAddr                 string        `toml:"listen_addr" split_words:"true"`
	TLSListenAddr              string        `toml:"tls_listen_addr" split_words:"true"`
	PrometheusListenAddr       string        `toml:"prometheus_listen_addr" split_words:"true"`
	BinDir                     string        `toml:"bin_dir"`
	Git                        Git           `toml:"git" envconfig:"git"`
	Storages                   []Storage     `toml:"storage" envconfig:"storage"`
	Logging                    Logging       `toml:"logging" envconfig:"logging"`
	Prometheus                 Prometheus    `toml:"prometheus"`
	Auth                       Auth          `toml:"auth"`
	TLS                        TLS           `toml:"tls"`
	Ruby                       Ruby          `toml:"gitaly-ruby"`
	GitlabShell                GitlabShell   `toml:"gitlab-shell"`
	Concurrency                []Concurrency `toml:"concurrency"`
	GracefulRestartTimeout     time.Duration
	GracefulRestartTimeoutToml duration `toml:"graceful_restart_timeout"`
	InternalSocketDir          string   `toml:"internal_socket_dir"`
}

// TLS configuration
type TLS struct {
	CertPath string `toml:"certificate_path"`
	KeyPath  string `toml:"key_path"`
}

// GitlabShell contains the settings required for executing `gitlab-shell`
type GitlabShell struct {
	Dir string `toml:"dir"`
}

// Git contains the settings for the Git executable
type Git struct {
	BinPath string `toml:"bin_path"`

	// ProtocolV2Enabled can be set to true to enable the newer Git protocol
	// version. This should not be enabled until GitLab *either* stops
	// using transfer.hideRefs for security purposes, *or* Git protocol v2
	// respects this setting:
	//
	// https://public-inbox.org/git/20181213155817.27666-1-avarab@gmail.com/T/
	//
	// This is not user-configurable. Once a new Git version has been released,
	// we can add code to enable it if the detected git binary is new enough
	ProtocolV2Enabled bool
}

// Storage contains a single storage-shard
type Storage struct {
	Name string
	Path string
}

// Logging contains the logging configuration for Gitaly
type Logging struct {
	Format            string
	SentryDSN         string `toml:"sentry_dsn"`
	RubySentryDSN     string `toml:"ruby_sentry_dsn"`
	SentryEnvironment string `toml:"sentry_environment"`
	Level             string `toml:"level"`
}

// Prometheus contains additional configuration data for prometheus
type Prometheus struct {
	GRPCLatencyBuckets []float64 `toml:"grpc_latency_buckets"`
}

// Concurrency allows endpoints to be limited to a maximum concurrency per repo
type Concurrency struct {
	RPC        string `toml:"rpc"`
	MaxPerRepo int    `toml:"max_per_repo"`
}

// Load initializes the Config variable from file and the environment.
//  Environment variables take precedence over the file.
func Load(file io.Reader) error {
	Config = config{}

	if _, err := toml.DecodeReader(file, &Config); err != nil {
		return fmt.Errorf("load toml: %v", err)
	}

	if err := envconfig.Process("gitaly", &Config); err != nil {
		return fmt.Errorf("envconfig: %v", err)
	}

	Config.setDefaults()

	return nil
}

// Validate checks the current Config for sanity.
func Validate() error {
	for _, err := range []error{
		validateListeners(),
		validateStorages(),
		validateToken(),
		SetGitPath(),
		validateShell(),
		ConfigureRuby(),
		validateBinDir(),
		validateInternalSocketDir(),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *config) setDefaults() {
	c.GracefulRestartTimeout = c.GracefulRestartTimeoutToml.Duration
	if c.GracefulRestartTimeout == 0 {
		c.GracefulRestartTimeout = 1 * time.Minute
	}
}

func validateListeners() error {
	if len(Config.SocketPath) == 0 && len(Config.ListenAddr) == 0 {
		return fmt.Errorf("invalid listener config: at least one of socket_path and listen_addr must be set")
	}
	return nil
}

func validateShell() error {
	if len(Config.GitlabShell.Dir) == 0 {
		return fmt.Errorf("gitlab-shell.dir is not set")
	}

	return validateIsDirectory(Config.GitlabShell.Dir, "gitlab-shell.dir")
}

func validateIsDirectory(path, name string) error {
	s, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !s.IsDir() {
		return fmt.Errorf("not a directory: %q", path)
	}

	log.WithField("dir", path).
		Debugf("%s set", name)

	return nil
}

func validateStorages() error {
	if len(Config.Storages) == 0 {
		return fmt.Errorf("no storage configurations found. Are you using the right format? https://gitlab.com/gitlab-org/gitaly/issues/397")
	}

	for i, storage := range Config.Storages {
		if storage.Name == "" {
			return fmt.Errorf("empty storage name in %v", storage)
		}

		if storage.Path == "" {
			return fmt.Errorf("empty storage path in %v", storage)
		}

		if fs, err := os.Stat(storage.Path); err != nil || !fs.IsDir() {
			return fmt.Errorf("storage paths have to exist %v", storage)
		}

		stPath := filepath.Clean(storage.Path)
		for j := 0; j < i; j++ {
			other := Config.Storages[j]
			if other.Name == storage.Name {
				return fmt.Errorf("storage %q is defined more than once", storage.Name)
			}

			otherPath := filepath.Clean(other.Path)
			if stPath == otherPath {
				// This is weird but we allow it for legacy gitlab.com reasons.
				continue
			}

			if strings.HasPrefix(stPath, otherPath) || strings.HasPrefix(otherPath, stPath) {
				// If storages have the same sub directory, that is allowed
				if filepath.Dir(stPath) == filepath.Dir(otherPath) {
					continue
				}
				return fmt.Errorf("storage paths may not nest: %q and %q", storage.Name, other.Name)
			}
		}
	}

	return nil
}

// SetGitPath populates the variable GitPath with the path to the `git`
// executable. It warns if no path was specified in the configuration.
func SetGitPath() error {
	if Config.Git.BinPath != "" {
		return nil
	}

	resolvedPath, err := exec.LookPath("git")
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"resolvedPath": resolvedPath,
	}).Warn("git path not configured. Using default path resolution")

	Config.Git.BinPath = resolvedPath

	return nil
}

// StoragePath looks up the base path for storageName. The second boolean
// return value indicates if anything was found.
func StoragePath(storageName string) (string, bool) {
	for _, storage := range Config.Storages {
		if storage.Name == storageName {
			return storage.Path, true
		}
	}
	return "", false
}

func validateBinDir() error {
	if err := validateIsDirectory(Config.BinDir, "bin_dir"); err != nil {
		log.WithError(err).Warn("Gitaly bin directory is not configured")
		// TODO this must become a fatal error
		return nil
	}

	var err error
	Config.BinDir, err = filepath.Abs(Config.BinDir)
	return err
}

func validateInternalSocketDir() error {
	dir := Config.InternalSocketDir
	// When empty, fall back to the `/tmp` directory
	if dir == "" {
		return nil
	}

	if !filepath.IsAbs(dir) {
		return fmt.Errorf("%s must be an absolute path", dir)
	}

	f, err := os.Lstat(dir)
	switch {
	case os.IsNotExist(err):
		return fmt.Errorf("InternalSocketDir %s does not exit", dir)
	case err != nil:
		return err
	case !f.IsDir():
		return fmt.Errorf("InternalSocketDir %s is not a directory", dir)
	case f.Mode().String() != "drwx------":
		return fmt.Errorf("InternalSocketDir %s has FileMode %s, should have mask 700", dir, f.Mode().String())
	}

	return nil
}
