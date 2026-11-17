package setup

import (
	"context"
	"fmt"
	"os"
	"reflect"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

func expandEnvHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if f.Kind() == reflect.String && t.Kind() == reflect.String {
			return os.ExpandEnv(data.(string)), nil
		}
		return data, nil
	}
}

func MainWrapper(ctx context.Context, svc Service, m func() error, opts ...ConfigOption) error {

	if err := InitViper(ctx, svc, opts...); err != nil {
		return err
	}

	var backends Backends
	if err := viper.UnmarshalKey("telemetry.backends", &backends, viper.DecodeHook(expandEnvHook())); err != nil {
		return err
	}

	cfg := TelemetryOptions{
		Service: svc,
		Backend: backends,
	}

	shutdown, err := Setup(ctx, cfg)
	if err != nil {
		fmt.Printf("setup telemetry failed: %v\n", err)
	}
	defer shutdown()

	return m()
}
