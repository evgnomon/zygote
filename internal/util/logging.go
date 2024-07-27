package util

import "go.uber.org/zap"

func Logger() (*zap.Logger, error) {
	logger, err := zap.NewProduction(zap.AddStacktrace(zap.ErrorLevel))
	if err != nil {
		return nil, err
	}
	defer func() {
		err = logger.Sync()
	}()
	return logger, nil
}
