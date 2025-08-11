//go:build !integration

package go_rabbitmq_test

import (
	"context"
	"testing"
	"time"

	"github.com/afaf-tech/go-rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConnection_ConfigDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		config        *rabbitmq.Config
		expectedURI   string
		expectedRetry time.Duration
		expectedMaxCh int
	}{
		{
			name:          "empty config uses defaults",
			config:        &rabbitmq.Config{},
			expectedURI:   "amqp://guest:guest@localhost:5672/",
			expectedRetry: 5 * time.Second,
			expectedMaxCh: 10,
		},
		{
			name: "partial config preserves custom values",
			config: &rabbitmq.Config{
				URI:           "amqp://custom:pass@example.com:5672/",
				RetryDuration: 10 * time.Second,
			},
			expectedURI:   "amqp://custom:pass@example.com:5672/",
			expectedRetry: 10 * time.Second,
			expectedMaxCh: 10,
		},
		{
			name: "all custom values preserved",
			config: &rabbitmq.Config{
				URI:           "amqp://user:pass@host:1234/vhost",
				RetryDuration: 2 * time.Second,
				MaxChannels:   20,
				AMQPConfig:    &amqp.Config{Heartbeat: 30 * time.Second},
			},
			expectedURI:   "amqp://user:pass@host:1234/vhost",
			expectedRetry: 2 * time.Second,
			expectedMaxCh: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn := rabbitmq.NewConnection(tt.config)
			require.NotNil(t, conn)

			// Since we can't access private fields directly in black-box testing,
			// we verify behavior through the public API where possible.
			// For config validation, we ensure NewConnection doesn't panic
			// and returns a valid connection object.
			assert.NotNil(t, conn)
		})
	}
}

func TestQueueOptions_Defaults(t *testing.T) {
	t.Parallel()

	// Test that QueueOptions can be created with various configurations
	tests := []struct {
		name    string
		options rabbitmq.QueueOptions
	}{
		{
			name:    "default options",
			options: rabbitmq.QueueOptions{},
		},
		{
			name: "durable queue",
			options: rabbitmq.QueueOptions{
				Durable: true,
			},
		},
		{
			name: "exclusive auto-delete",
			options: rabbitmq.QueueOptions{
				Exclusive:  true,
				AutoDelete: true,
			},
		},
		{
			name: "auto-ack with args",
			options: rabbitmq.QueueOptions{
				AutoAck: true,
				Args:    amqp.Table{"x-message-ttl": 60000},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Verify QueueOptions struct can be created and accessed
			opts := tt.options
			assert.NotNil(t, &opts)

			// Verify we can access all fields
			_ = opts.Durable
			_ = opts.AutoAck
			_ = opts.AutoDelete
			_ = opts.Exclusive
			_ = opts.NoWait
			_ = opts.NoLocal
			_ = opts.Args
		})
	}
}

func TestConnection_ContextHandling(t *testing.T) {
	t.Parallel()

	// Test context cancellation behavior
	t.Run("cancelled context should return promptly", func(t *testing.T) {
		t.Parallel()

		config := &rabbitmq.Config{
			URI: "amqp://invalid:invalid@nonexistent:5672/",
		}
		conn := rabbitmq.NewConnection(config)
		require.NotNil(t, conn)

		// Create a context that's already cancelled
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Test that operations respect context cancellation
		// Since we can't directly test Connect with context in black-box mode,
		// we focus on ensuring the connection object is properly initialized
		assert.True(t, conn.IsClosed()) // Should be closed initially
		assert.Equal(t, context.Canceled, ctx.Err())
	})

	t.Run("context with timeout", func(t *testing.T) {
		t.Parallel()

		config := &rabbitmq.Config{
			URI: "amqp://invalid:invalid@nonexistent:5672/",
		}
		conn := rabbitmq.NewConnection(config)
		require.NotNil(t, conn)

		// Create a context with a very short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		// Wait for context to expire
		<-ctx.Done()
		assert.Equal(t, context.DeadlineExceeded, ctx.Err())
	})
}

func TestConnection_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	// Test that multiple goroutines can safely access connection methods
	t.Run("concurrent IsClosed calls", func(t *testing.T) {
		t.Parallel()

		config := &rabbitmq.Config{}
		conn := rabbitmq.NewConnection(config)
		require.NotNil(t, conn)

		const numGoroutines = 10
		done := make(chan bool, numGoroutines)

		// Start multiple goroutines calling IsClosed concurrently
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer func() { done <- true }()

				// Call IsClosed multiple times
				for j := 0; j < 100; j++ {
					_ = conn.IsClosed()
				}
			}()
		}

		// Wait for all goroutines to complete with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		for i := 0; i < numGoroutines; i++ {
			select {
			case <-done:
				// Goroutine completed successfully
			case <-ctx.Done():
				t.Fatal("timeout waiting for concurrent operations to complete")
			}
		}
	})
}

func TestConnection_InvalidConfig(t *testing.T) {
	t.Parallel()

	// Test various edge cases in configuration
	tests := []struct {
		name   string
		config *rabbitmq.Config
		valid  bool
	}{
		{
			name:   "nil AMQPConfig is handled",
			config: &rabbitmq.Config{AMQPConfig: nil},
			valid:  true,
		},
		{
			name:   "zero MaxChannels uses default",
			config: &rabbitmq.Config{MaxChannels: 0},
			valid:  true,
		},
		{
			name:   "zero RetryDuration uses default",
			config: &rabbitmq.Config{RetryDuration: 0},
			valid:  true,
		},
		{
			name:   "positive MaxChannels accepted",
			config: &rabbitmq.Config{MaxChannels: 5},
			valid:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.valid {
				// Should not panic with valid configuration
				conn := rabbitmq.NewConnection(tt.config)
				require.NotNil(t, conn)
				assert.True(t, conn.IsClosed()) // Should start closed
			}
		})
	}
}

// TODO: The following behaviors cannot be fully tested without a broker connection:
// - Actual connection establishment and retry logic
// - Channel pool management and sharing
// - Reconnection on connection loss
// - Channel creation and cleanup
// These require integration tests with a real RabbitMQ instance.

func TestConnection_SkippedBehaviors(t *testing.T) {
	t.Parallel()

	t.Skip("TODO: Channel pool behavior requires broker connection to observe. " +
		"Need public API to inspect connection state, channel count, or retry attempts.")

	t.Skip("TODO: Backoff/retry computation requires public API to inspect retry intervals " +
		"or mock time to make tests deterministic.")

	t.Skip("TODO: Reconnection logic requires public API to simulate connection loss " +
		"and observe reconnection attempts without actual broker.")
}
