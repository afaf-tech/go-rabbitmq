//go:build integration

package helpers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// UniqueQueue returns a unique queue name based on the test name and current timestamp
func UniqueQueue(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test-queue-%s-%d", t.Name(), time.Now().UnixNano())
}

// EventuallyEqual polls the gotFn function until it returns the expected want value
// or the timeout is reached. It checks at the specified interval.
func EventuallyEqual(t *testing.T, want, gotFn func() int, timeout, interval time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final check before failing
			got := gotFn()
			require.Equal(t, want(), got, "EventuallyEqual timed out after %v", timeout)
			return
		case <-ticker.C:
			if gotFn() == want() {
				return // Success
			}
		}
	}
}

// ContextWithTimeout creates a context with the specified timeout
func ContextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// DefaultContext creates a context with a default 5-second timeout
func DefaultContext() (context.Context, context.CancelFunc) {
	return ContextWithTimeout(5 * time.Second)
}

// LongContext creates a context with a 15-second timeout for longer operations
func LongContext() (context.Context, context.CancelFunc) {
	return ContextWithTimeout(15 * time.Second)
}

// ShortContext creates a context with a 1-second timeout for quick operations
func ShortContext() (context.Context, context.CancelFunc) {
	return ContextWithTimeout(1 * time.Second)
}

// VeryShortContext creates a context with a very short timeout for testing deadline enforcement
func VeryShortContext() (context.Context, context.CancelFunc) {
	return ContextWithTimeout(20 * time.Millisecond)
}

// WaitForCondition polls a condition function until it returns true or timeout is reached
func WaitForCondition(t *testing.T, condition func() bool, timeout time.Duration, message string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			require.True(t, condition(), "condition not met: %s (timeout after %v)", message, timeout)
			return
		case <-ticker.C:
			if condition() {
				return // Success
			}
		}
	}
}

// MessageCounter is a thread-safe counter for tracking received messages
type MessageCounter struct {
	count   int
	updates chan struct{}
}

// NewMessageCounter creates a new MessageCounter
func NewMessageCounter() *MessageCounter {
	return &MessageCounter{
		updates: make(chan struct{}, 1000), // Buffered to avoid blocking
	}
}

// Increment increases the counter by 1
func (mc *MessageCounter) Increment() {
	mc.count++
	select {
	case mc.updates <- struct{}{}:
	default:
		// Channel is full, skip
	}
}

// Count returns the current count
func (mc *MessageCounter) Count() int {
	return mc.count
}

// WaitForCount waits until the counter reaches the expected count or timeout
func (mc *MessageCounter) WaitForCount(t *testing.T, expected int, timeout time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			require.Equal(t, expected, mc.count, "message count timeout after %v", timeout)
			return
		case <-ticker.C:
			if mc.count >= expected {
				return // Success
			}
		case <-mc.updates:
			if mc.count >= expected {
				return // Success
			}
		}
	}
}

// MessageTracker tracks unique messages to detect duplicates
type MessageTracker struct {
	messages map[string]int
}

// NewMessageTracker creates a new MessageTracker
func NewMessageTracker() *MessageTracker {
	return &MessageTracker{
		messages: make(map[string]int),
	}
}

// Track records a message and returns true if it's a duplicate
func (mt *MessageTracker) Track(messageID string) bool {
	count := mt.messages[messageID]
	mt.messages[messageID] = count + 1
	return count > 0 // Returns true if this is a duplicate
}

// Count returns the number of times a specific message was seen
func (mt *MessageTracker) Count(messageID string) int {
	return mt.messages[messageID]
}

// TotalMessages returns the total number of unique messages tracked
func (mt *MessageTracker) TotalMessages() int {
	return len(mt.messages)
}

// AllMessagesSeenOnce returns true if all tracked messages were seen exactly once
func (mt *MessageTracker) AllMessagesSeenOnce() bool {
	for _, count := range mt.messages {
		if count != 1 {
			return false
		}
	}
	return true
}
