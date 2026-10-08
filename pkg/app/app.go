// Package app defines common utility functions for building an internal.
package app

import (
	"context"
	"fmt"
	_ "io"
	"os"
	"os/signal"
	_ "reflect"
	"strings"
	"syscall"

	"github.com/asaskevich/govalidator"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/urfave/sflags"
	"github.com/urfave/sflags/gen/gpflag"
	"go.uber.org/zap"
)

// New creates new Cobra internal and sets up root logger.
// It supports graceful shutdown: the returned context is canceled if SIGTERM or SIGINT are received.
// Use the returned context as the root context in the internal.
// It will panic if logger creation fails.
// The caller must call .Sync() on the logger at the end of the internal lifecycle.
func New(name string, cfg interface{}, opt ...Option) (*cobra.Command, *zap.Logger, context.Context) {
	var opts options
	for _, o := range opt {
		o(&opts)
	}

	log, err := NewZapLogger(opts.LogOpts...)
	if err != nil {
		panic(err)
	}

	cmd := Bootstrap(cfg, &cobra.Command{
		Use: name,
	})

	ctx, cancel := context.WithCancel(context.Background())

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)

	cleanups := []func(){
		cancel,
	}

	go func() {
		sig := <-ch
		log.Named("app").Info("Shutdown signal received", zap.String("signal", sig.String()))
		signal.Stop(ch)
		for _, c := range cleanups {
			if c != nil {
				c()
			}
		}
	}()

	return cmd, log, ctx
}

// Bootstrap sets up the internal configuration and reads it from the environment and config file.
func Bootstrap(cfg interface{}, cmd *cobra.Command) *cobra.Command {
	v := viper.New()
	if err := gpflag.ParseTo(cfg, cmd.PersistentFlags(), sflags.FlagDivider("."), sflags.FlagTag("mapstructure")); err != nil {
		panic(err)
	}

	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	if err := v.BindPFlags(cmd.PersistentFlags()); err != nil {
		panic(err)
	}

	cmd.SetGlobalNormalizationFunc(func(fs *pflag.FlagSet, name string) pflag.NormalizedName {
		return pflag.NormalizedName(strings.ReplaceAll(name, "_", "-"))
	})

	var cfgFile string

	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "path to the config file")

	cmd.SilenceUsage = true
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if cfgFile != "" {
			v.SetConfigFile(cfgFile)
		} else {
			v.SetConfigName(cmd.Name())
			v.AddConfigPath(".")
			v.AddConfigPath("/etc/")
		}

		if err := v.ReadInConfig(); err == nil {
			_, _ = fmt.Fprintln(os.Stderr, "Using config file:", v.ConfigFileUsed())
		}

		if err := v.Unmarshal(cfg); err != nil {
			return err
		}

		if _, err := govalidator.ValidateStruct(cfg); err != nil {
			return err
		}

		return nil
	}

	return cmd
}
