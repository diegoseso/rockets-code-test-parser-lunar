package app

import (
	"io"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/crypto/ssh/terminal"
)

// NewZapLogger creates new Zap logger configured for Google Cloud Stackdriver.
func NewZapLogger(opts ...zap.Option) (*zap.Logger, error) {
	l, err := newZapConfig().Build(opts...)
	if err != nil {
		return nil, err
	}

	zap.RedirectStdLog(l)

	return l, nil
}

func newZapConfig() zap.Config {
	var cfg zap.Config

	if isTerminal(os.Stderr) {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		cfg = zap.NewProductionConfig()
	}

	cfg.EncoderConfig.LevelKey = "severity"
	cfg.EncoderConfig.MessageKey = "message"
	cfg.EncoderConfig.TimeKey = "time"
	cfg.Sampling = nil

	return cfg
}

func isTerminal(w io.Writer) bool {
	switch v := w.(type) {
	case *os.File:
		return terminal.IsTerminal(int(v.Fd()))
	default:
		return false
	}
}
