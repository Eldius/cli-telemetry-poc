package setup

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

var (
	ErrEmptyAppName        = errors.New("empty service name")
	ErrViperInitialization = errors.New("viper initialization error")
)

type ConfigOptions struct {
	DefaultConfigs              map[string]any
	ConfigFileLookupDirectories []string
	EnvPrefix                   string
	ConfigFile                  string
	ConfigFileName              string
	ConfigFileType              string
}

type ConfigOption func(*ConfigOptions)

// WithConfigFileLookupDirectories defines directories to lookup for
// config file
func WithConfigFileLookupDirectories(dirs ...string) ConfigOption {
	return func(v *ConfigOptions) {
		v.ConfigFileLookupDirectories = dirs
	}
}

// WithDefaultConfigs defines default Viper values
func WithDefaultConfigs(configs map[string]any) ConfigOption {
	return func(v *ConfigOptions) {
		v.DefaultConfigs = configs
	}
}

// WithEnvPrefix defines the environment variables'
// name prefix for properties lookup
func WithEnvPrefix(prefix string) ConfigOption {
	return func(v *ConfigOptions) {
		v.EnvPrefix = prefix
	}
}

func WithConfigFile(path string) ConfigOption {
	return func(v *ConfigOptions) {
		v.ConfigFile = path
	}
}

func WithConfigFileName(name string) ConfigOption {
	return func(v *ConfigOptions) {
		v.ConfigFileName = name
	}
}

func WithConfigFileType(t string) ConfigOption {
	return func(v *ConfigOptions) {
		v.ConfigFileType = t
	}
}

func InitViper(ctx context.Context, svc Service, opts ...ConfigOption) error {
	cfg := ConfigOptions{}
	for _, opt := range opts {
		opt(&cfg)
	}

	if svc.Name == "" {
		return fmt.Errorf("invalid app name: %w", ErrEmptyAppName)
	}

	if cfg.ConfigFile != "" {
		// Use the config file from the flag.
		viper.SetConfigFile(cfg.ConfigFile)
	} else {
		// Find a home directory.
		home, err := homedir.Dir()
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}

		for _, dir := range cfg.ConfigFileLookupDirectories {
			viper.AddConfigPath(dir)
		}
		viper.AddConfigPath(filepath.Join(home, fmt.Sprintf(".%s", svc.Name)))
		viper.AddConfigPath(filepath.Join(home))
		viper.SetConfigType("yaml")
		viper.SetConfigName(cfg.ConfigFileName)
		viper.SetConfigType(cfg.ConfigFileType)
	}

	for k, v := range cfg.DefaultConfigs {
		viper.SetDefault(k, v)
	}
	viper.SetEnvPrefix(cfg.EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		log.Println("Using config file:", viper.ConfigFileUsed())
	} else {
		err = fmt.Errorf("%w: %w", ErrViperInitialization, err)
		log.Printf("Could not find config file using default values: %s", err)

		return err
	}
	return nil
}
