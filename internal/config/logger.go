package config

import (
	"os"

	log "github.com/sirupsen/logrus"
)

var (
	debugLoggingEnabled = os.Getenv("GITALY_DEBUG") == "1"
)

func init() {
	// This ensures that any log statements that occur before
	// the configuration has been loaded will be written to
	// stdout instead of stderr
	log.SetOutput(os.Stdout)
}

func configureLoggingFormat() {
	switch Config.Logging.Format {
	case "json":
		log.SetFormatter(&log.JSONFormatter{})
		return
	case "":
		// Just stick with the default
		return
	default:
		log.WithField("format", Config.Logging.Format).Fatal("invalid logger format")
	}
}

// ConfigureLogging uses the global conf and environmental vars to configure the logged
func ConfigureLogging() {
	if level, err := log.ParseLevel(Config.Logging.Level); err != nil {
		log.SetLevel(log.InfoLevel)
	} else {
		log.SetLevel(level)
	}

	// Allow override based on environment variable
	if debugLoggingEnabled {
		log.SetLevel(log.DebugLevel)
	}

	configureLoggingFormat()
}
