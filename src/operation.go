package main

import (
	"fmt"
	"time"
)

// Operation is a function type that represents an operation that might fail and need a retry.
type Operation func() error

const (
	maxRetries     = 5
	initialBackoff = 300 * time.Millisecond
)

// ExponentialBackoff retries the provided operation with an exponential backoff strategy.
func ExponentialBackoff(operation Operation, maxRetries int, initialBackoff time.Duration) error {
	backoff := initialBackoff
	var err error
	for i := 0; i < maxRetries; i++ {
		err = operation()
		if err == nil {
			return nil // Success
		}

		if i < maxRetries-1 {
			// If not the last retry, wait for a bit
			time.Sleep(backoff)
			backoff *= 2 // Exponential increase
		}
	}

	return fmt.Errorf("operation failed after %d retries, returned error: %s", maxRetries, err)
}
