package util

import (
	"os"

	"go.uber.org/zap"
)

func Logger() (*zap.Logger, error) {
	logger, err := zap.NewProduction(zap.AddStacktrace(zap.ErrorLevel))
	if os.Getenv("Z_LOGGER") == "" {
		logger, err = zap.NewDevelopment(zap.AddStacktrace(zap.ErrorLevel))
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		err = logger.Sync()
	}()
	return logger, nil
}
