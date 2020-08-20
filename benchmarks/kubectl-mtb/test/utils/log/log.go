package log

import (
	"log"

	"go.uber.org/zap"
)

// Log represents global logger
var Logging *zap.SugaredLogger

func zapLogger(debug bool) {
	var err error
	var logger *zap.Logger

	if debug {
		logger, err = zap.NewDevelopment()
		Logging = logger.Sugar()
	} else {
		logger, err = zap.NewProduction()
		Logging = logger.Sugar()
	}

	// who watches the watchmen?
	fatalIfErr(err, log.Fatalf)
}

func fatalIfErr(err error, f func(format string, v ...interface{})) {
	if err != nil {
		f("unable to construct the logger: %v", err)
	}
}

// SetupLogger setups global logger
func SetupLogger(debug bool) {
	zapLogger(debug)
}
