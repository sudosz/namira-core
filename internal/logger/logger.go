package logger

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	logger  *zap.Logger
	once    sync.Once
	initErr error
)

func Init(level string) (*zap.Logger, error) {
	once.Do(func() {
		var cfg zap.Config

		var logLevel zapcore.Level
		switch level {
		case "debug":
			logLevel = zapcore.DebugLevel
		case "info":
			logLevel = zapcore.InfoLevel
		case "warn":
			logLevel = zapcore.WarnLevel
		case "error":
			logLevel = zapcore.ErrorLevel
		default:
			logLevel = zapcore.InfoLevel
		}

		encoderConfig := zap.NewProductionEncoderConfig()
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		encoderConfig.StacktraceKey = ""

		cfg = zap.Config{
			Level:       zap.NewAtomicLevelAt(logLevel),
			Development: false,
			Sampling: &zap.SamplingConfig{
				Initial:    100,
				Thereafter: 100,
			},
			Encoding:         "json",
			EncoderConfig:    encoderConfig,
			OutputPaths:      []string{"stderr"},
			ErrorOutputPaths: []string{"stderr"},
		}

		logger, initErr = cfg.Build()
		if initErr != nil {
			initErr = fmt.Errorf("failed to initialize logger: %w", initErr)
		}
	})

	return logger, initErr
}

func Get() (*zap.Logger, error) {
	if logger == nil {
		_, err := Init("info")
		if err != nil {
			return nil, err
		}
	}
	return logger, nil
}

func MustGet() *zap.Logger {
	l, err := Get()
	if err != nil {
		panic(err)
	}
	return l
}

func Sync() error {
	if logger != nil {
		return logger.Sync()
	}
	return nil
}

func Debug(msg string, fields ...zap.Field) {
	if l, err := Get(); err == nil {
		l.Debug(msg, fields...)
	}
}

func Info(msg string, fields ...zap.Field) {
	if l, err := Get(); err == nil {
		l.Info(msg, fields...)
	}
}

func Warn(msg string, fields ...zap.Field) {
	if l, err := Get(); err == nil {
		l.Warn(msg, fields...)
	}
}

func Error(msg string, fields ...zap.Field) {
	if l, err := Get(); err == nil {
		l.Error(msg, fields...)
	}
}

func Fatal(msg string, fields ...zap.Field) {
	if l, err := Get(); err == nil {
		l.Fatal(msg, fields...)
	}
}

func With(fields ...zap.Field) *zap.Logger {
	if l, err := Get(); err == nil {
		return l.With(fields...)
	}
	return zap.NewNop()
}
