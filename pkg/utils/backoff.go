package utils

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/evgnomon/zygote/internal/util"
)

// BackoffConfig defines the configuration for exponential backoff
type BackoffConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

// ExponentialBackoff executes a function with exponential backoff retry
func (config BackoffConfig) Retry(ctx context.Context, fn func() error) error {
	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		// Log the error for this attempt
		logger.Debug("Backoff attempt failed", util.M{
			"attempt": attempt + 1,
			"error":   err.Error(),
		})

		// Don't wait on the last attempt
		if attempt < config.MaxAttempts-1 {
			delay := time.Duration(math.Pow(2, float64(attempt))) * config.InitialDelay
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		return fmt.Errorf("operation failed after %d attempts: %w", config.MaxAttempts, err)
	}
	return fmt.Errorf("operation failed after %d attempts", config.MaxAttempts)
}
