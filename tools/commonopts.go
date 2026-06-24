package tools

import (
	"flag"
	"log/slog"
	"os"

	"github.com/Kuniwak/ttx/slograw"
)

type CommonOptions struct {
	Help     bool         `json:"help"`
	Version  bool         `json:"version"`
	LogLevel slog.Level   `json:"log_level"`
	Logger   *slog.Logger `json:"logger"`
}

var VersionCommonOptions = &CommonOptions{Version: true}
var HelpCommonOptions = &CommonOptions{Help: true}

type CommonRawOptions struct {
	Help         bool
	ShortVersion bool
	Version      bool
	Silent       bool
	Debug        bool
}

func DeclareCommonOptions(flags *flag.FlagSet, options *CommonRawOptions) {
	flags.BoolVar(&options.ShortVersion, "v", false, "show version")
	flags.BoolVar(&options.Version, "version", false, "show version")
	flags.BoolVar(&options.Silent, "silent", false, "silent mode")
	flags.BoolVar(&options.Debug, "debug", false, "debug mode")
}

func ValidateCommonOptions(options *CommonRawOptions) (*CommonOptions, error) {
	if options.ShortVersion || options.Version {
		return &CommonOptions{Version: true}, nil
	}

	var logLevel slog.Level
	if options.Debug {
		logLevel = slog.LevelDebug
	} else if options.Silent {
		logLevel = slog.LevelError
	} else {
		logLevel = slog.LevelInfo
	}

	logger := slog.New(slograw.NewHandler(os.Stderr, logLevel))

	return &CommonOptions{
		LogLevel: logLevel,
		Logger:   logger,
	}, nil
}
