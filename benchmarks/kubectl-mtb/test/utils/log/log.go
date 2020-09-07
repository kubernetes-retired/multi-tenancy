package log

import (
	"go.uber.org/zap"
)

func GetLogger(debug bool) *zap.SugaredLogger {
	if debug {
		logger, _ := zap.NewDevelopment()
		return logger.Sugar()
	}
	logger, _ := zap.NewProduction()
	return logger.Sugar()
}
