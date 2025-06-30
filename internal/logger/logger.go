package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	logger  *zap.Logger
	once    sync.Once
	initErr error
)

// Config holds logger configuration
type Config struct {
	Level         zapcore.Level
	ConsoleOutput bool
	FileOutput    bool
	Filename      string
	MaxSize       int  // megabytes
	MaxAge        int  // days
	MaxBackups    int  // number of backups to keep
	Compress      bool // compress rotated files
	JSONFormat    bool // use JSON format for console output
}

const (
	DefaultFilename   = "logs/app.log"
	DefaultMaxSize    = 100 // megabytes
	DefaultMaxAge     = 30  // days
	DefaultMaxBackups = 10
	DefaultCompress   = true
)

// Option is a function that configures the logger
type Option func(*Config)

// WithLevel sets the logging level
func WithLevel(level string) Option {
	return func(c *Config) {
		switch level {
		case "debug":
			c.Level = zapcore.DebugLevel
		case "info":
			c.Level = zapcore.InfoLevel
		case "warn":
			c.Level = zapcore.WarnLevel
		case "error":
			c.Level = zapcore.ErrorLevel
		case "fatal":
			c.Level = zapcore.FatalLevel
		case "panic":
			c.Level = zapcore.PanicLevel
		default:
			c.Level = zapcore.InfoLevel
		}
	}
}

// WithConsoleOutput enables/disables console output
func WithConsoleOutput(enabled bool) Option {
	return func(c *Config) { c.ConsoleOutput = enabled }
}

// WithFileOutput enables/disables file output
func WithFileOutput(enabled bool) Option {
	return func(c *Config) { c.FileOutput = enabled }
}

// WithFilename sets the log filename
func WithFilename(filename string) Option {
	return func(c *Config) { c.Filename = filename }
}

// WithJSONFormat enables JSON format for console output (for API mode)
func WithJSONFormat(enabled bool) Option {
	return func(c *Config) { c.JSONFormat = enabled }
}

// WithRotationConfig sets the log rotation configuration
func WithRotationConfig(maxSize, maxAge, maxBackups int, compress bool) Option {
	return func(c *Config) {
		c.MaxSize = maxSize
		c.MaxAge = maxAge
		c.MaxBackups = maxBackups
		c.Compress = compress
	}
}

// Init initializes logger with default options
func Init(level string) (*zap.Logger, error) {
	return InitWithOptions(WithLevel(level))
}

// InitForCLI initializes logger for CLI with human-readable console output
func InitForCLI(level string) (*zap.Logger, error) {
	return InitWithOptions(
		WithLevel(level),
		WithConsoleOutput(true),
		WithJSONFormat(false),
	)
}

// InitForAPI initializes logger for API with JSON console output
func InitForAPI(level string, enableFileLogging bool) (*zap.Logger, error) {
	return InitWithOptions(
		WithLevel(level),
		WithConsoleOutput(true),
		WithJSONFormat(true),
		WithFileOutput(enableFileLogging),
	)
}

// InitWithOptions initializes logger with options
func InitWithOptions(opts ...Option) (*zap.Logger, error) {
	once.Do(func() {
		// Default config
		config := &Config{
			Level:         zapcore.InfoLevel,
			ConsoleOutput: true,
			FileOutput:    false,
			Filename:      DefaultFilename,
			MaxSize:       DefaultMaxSize,
			MaxAge:        DefaultMaxAge,
			MaxBackups:    DefaultMaxBackups,
			Compress:      DefaultCompress,
			JSONFormat:    false,
		}

		// Apply options
		for _, opt := range opts {
			opt(config)
		}

		var cores []zapcore.Core

		// Configure console output
		if config.ConsoleOutput {
			var consoleEncoder zapcore.Encoder

			if config.JSONFormat {
				// JSON format for API (structured logging)
				jsonConfig := zap.NewProductionEncoderConfig()
				jsonConfig.EncodeTime = zapcore.ISO8601TimeEncoder
				jsonConfig.StacktraceKey = ""
				consoleEncoder = zapcore.NewJSONEncoder(jsonConfig)
			} else {
				// Human-readable format for CLI
				consoleConfig := zap.NewDevelopmentEncoderConfig()
				consoleConfig.EncodeTime = zapcore.RFC3339TimeEncoder
				consoleConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
				consoleConfig.EncodeCaller = zapcore.ShortCallerEncoder
				consoleEncoder = zapcore.NewConsoleEncoder(consoleConfig)
			}

			consoleCore := zapcore.NewCore(
				consoleEncoder,
				zapcore.AddSync(os.Stdout),
				config.Level,
			)
			cores = append(cores, consoleCore)
		}

		// Configure file output with rotation
		if config.FileOutput {
			// Ensure logs directory exists
			if err := os.MkdirAll(filepath.Dir(config.Filename), 0755); err != nil {
				initErr = fmt.Errorf("failed to create logs directory: %w", err)
				return
			}

			fileEncoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
				TimeKey:      "ts",
				LevelKey:     "level",
				NameKey:      "logger",
				CallerKey:    "caller",
				MessageKey:   "msg",
				EncodeLevel:  zapcore.LowercaseLevelEncoder,
				EncodeTime:   zapcore.ISO8601TimeEncoder,
				EncodeCaller: zapcore.ShortCallerEncoder,
			})

			cores = append(cores, zapcore.NewCore(
				fileEncoder,
				zapcore.AddSync(&lumberjack.Logger{
					Filename:   config.Filename,
					MaxSize:    config.MaxSize,
					MaxAge:     config.MaxAge,
					MaxBackups: config.MaxBackups,
					Compress:   config.Compress,
				}),
				config.Level,
			))
		}

		if len(cores) == 0 {
			initErr = fmt.Errorf("no output configured for logger")
			return
		}

		logger = zap.New(zapcore.NewTee(cores...), zap.AddCaller())
	})

	return logger, initErr
}

// Get returns the logger instance
func Get() *zap.Logger {
	if logger == nil {
		logger, _ = Init("info")
	}
	return logger
}

// Sync flushes any buffered log entries
func Sync() error {
	if logger != nil {
		return logger.Sync()
	}
	return nil
}

func Debug(msg string, fields ...zap.Field) { Get().Debug(msg, fields...) }
func Info(msg string, fields ...zap.Field)  { Get().Info(msg, fields...) }
func Warn(msg string, fields ...zap.Field)  { Get().Warn(msg, fields...) }
func Error(msg string, fields ...zap.Field) { Get().Error(msg, fields...) }
func Fatal(msg string, fields ...zap.Field) { Get().Fatal(msg, fields...) }
func With(fields ...zap.Field) *zap.Logger  { return Get().With(fields...) }
