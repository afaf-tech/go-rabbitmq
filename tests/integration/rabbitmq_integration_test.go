//go:build integration

package go_rabbitmq_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/afaf-tech/go-rabbitmq"
	"github.com/afaf-tech/go-rabbitmq/tests/integration/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Short() {
		fmt.Println("Skipping integration tests in short mode")
		return
	}

	os.Exit(m.Run())
}

func TestPublishConsumeHappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	// Start RabbitMQ for this test
	pool, resource, amqpURL := helpers.StartRabbitMQ(t)
	defer helpers.StopRabbitMQ(t, pool, resource)

	queueName := helpers.UniqueQueue(t)
	ctx, cancel := helpers.LongContext()
	defer cancel()

	t.Logf("Testing publish/consume happy path with queue: %s", queueName)

	// Create connection
	config := &rabbitmq.Config{URI: amqpURL}
	conn := rabbitmq.NewConnection(config)
	require.NotNil(t, conn)

	err := conn.Connect()
	require.NoError(t, err)
	defer conn.Close()

	// Create producer
	producer, err := rabbitmq.NewProducer(conn, "test-producer", queueName)
	require.NoError(t, err)
	defer producer.Close()

	// Create consumer
	consumerOpts := rabbitmq.QueueOptions{
		Durable: true,
		AutoAck: false, // Manual ack for testing
	}
	consumer, err := rabbitmq.NewConsumer(conn, "test-consumer", queueName, consumerOpts)
	require.NoError(t, err)

	// Track received messages
	messageTracker := helpers.NewMessageTracker()
	messageCounter := helpers.NewMessageCounter()
	var mu sync.Mutex

	// Start consumer in background
	go func() {
		consumer.Start(ctx, func(msgCtx context.Context, msg *rabbitmq.Message) {
			mu.Lock()
			defer mu.Unlock()

			messageID := string(msg.Body)
			t.Logf("Received message: %s", messageID)

			isDuplicate := messageTracker.Track(messageID)
			assert.False(t, isDuplicate, "received duplicate message: %s", messageID)

			messageCounter.Increment()
			msg.Ack() // Acknowledge the message
		})
	}()

	// Wait a bit for consumer to start
	time.Sleep(100 * time.Millisecond)

	// Publish N messages
	const numMessages = 50
	for i := 0; i < numMessages; i++ {
		messageID := fmt.Sprintf("message-%d", i)
		publishOpts := rabbitmq.PublishOptions{
			Exchange:    "",
			RoutingKey:  queueName,
			ContentType: "text/plain",
			Body:        []byte(messageID),
		}
		err := producer.Publish(ctx, []byte(messageID), publishOpts)
		require.NoError(t, err)
		t.Logf("Published message: %s", messageID)
	}

	// Wait for all messages to be received
	messageCounter.WaitForCount(t, numMessages, 10*time.Second)

	// Verify all messages were received exactly once
	assert.Equal(t, numMessages, messageTracker.TotalMessages())
	assert.True(t, messageTracker.AllMessagesSeenOnce(), "not all messages received exactly once")
}

func TestContextDeadlineHonored(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	// Start RabbitMQ for this test
	pool, resource, amqpURL := helpers.StartRabbitMQ(t)
	defer helpers.StopRabbitMQ(t, pool, resource)

	queueName := helpers.UniqueQueue(t)
	t.Logf("Testing context deadline with queue: %s", queueName)

	// Create connection
	config := &rabbitmq.Config{URI: amqpURL}
	conn := rabbitmq.NewConnection(config)
	require.NotNil(t, conn)

	err := conn.Connect()
	require.NoError(t, err)
	defer conn.Close()

	// Create producer
	producer, err := rabbitmq.NewProducer(conn, "test-producer", queueName)
	require.NoError(t, err)
	defer producer.Close()

	// Test with very short context timeout
	ctx, cancel := helpers.VeryShortContext()
	defer cancel()

	start := time.Now()
	publishOpts := rabbitmq.PublishOptions{
		Exchange:    "",
		RoutingKey:  queueName,
		ContentType: "text/plain",
		Body:        []byte("test-message"),
	}
	err = producer.Publish(ctx, []byte("test-message"), publishOpts)
	elapsed := time.Since(start)

	// Should complete quickly (either success or timeout)
	assert.Less(t, elapsed, 100*time.Millisecond, "publish should complete quickly")

	// If it timed out, error should be context-related
	if err != nil {
		assert.Contains(t, err.Error(), "context", "error should be context-related")
	}
}

func TestNackWithoutRequeue(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	// Start RabbitMQ for this test
	pool, resource, amqpURL := helpers.StartRabbitMQ(t)
	defer helpers.StopRabbitMQ(t, pool, resource)

	queueName := helpers.UniqueQueue(t)
	ctx, cancel := helpers.LongContext()
	defer cancel()

	t.Logf("Testing nack without requeue with queue: %s", queueName)

	// Create connection
	config := &rabbitmq.Config{URI: amqpURL}
	conn := rabbitmq.NewConnection(config)
	require.NotNil(t, conn)

	err := conn.Connect()
	require.NoError(t, err)
	defer conn.Close()

	// Create producer
	producer, err := rabbitmq.NewProducer(conn, "test-producer", queueName)
	require.NoError(t, err)
	defer producer.Close()

	// Create consumer
	consumerOpts := rabbitmq.QueueOptions{Durable: true, AutoAck: false}
	consumer, err := rabbitmq.NewConsumer(conn, "test-consumer", queueName, consumerOpts)
	require.NoError(t, err)

	messageReceived := false
	redeliveryReceived := false

	// Start consumer in background
	go func() {
		consumer.Start(ctx, func(msgCtx context.Context, msg *rabbitmq.Message) {
			messageID := string(msg.Body)
			t.Logf("Received message: %s", messageID)

			if !messageReceived {
				messageReceived = true
				t.Logf("Nacking message without requeue: %s", messageID)
				msg.Nack() // Nack without requeue (should discard message)
			} else {
				redeliveryReceived = true
				t.Logf("Unexpected redelivery: %s", messageID)
				msg.Ack()
			}
		})
	}()

	// Wait for consumer to start
	time.Sleep(100 * time.Millisecond)

	// Publish a message
	publishOpts := rabbitmq.PublishOptions{
		Exchange:    "",
		RoutingKey:  queueName,
		ContentType: "text/plain",
		Body:        []byte("nack-test-message"),
	}
	err = producer.Publish(ctx, []byte("nack-test-message"), publishOpts)
	require.NoError(t, err)

	// Wait to ensure message is processed
	helpers.WaitForCondition(t, func() bool { return messageReceived }, 5*time.Second, "message should be received")

	// Wait a bit more to ensure no redelivery occurs
	time.Sleep(2 * time.Second)

	assert.True(t, messageReceived, "message should have been received")
	assert.False(t, redeliveryReceived, "message should not be redelivered after nack without requeue")
}

func TestGracefulShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	// Start RabbitMQ for this test
	pool, resource, amqpURL := helpers.StartRabbitMQ(t)
	defer helpers.StopRabbitMQ(t, pool, resource)

	queueName := helpers.UniqueQueue(t)
	ctx, cancel := helpers.LongContext()
	defer cancel()

	t.Logf("Testing graceful shutdown with queue: %s", queueName)

	// Create connection
	config := &rabbitmq.Config{URI: amqpURL}
	conn := rabbitmq.NewConnection(config)
	require.NotNil(t, conn)

	err := conn.Connect()
	require.NoError(t, err)

	// Create producer and consumer
	producer, err := rabbitmq.NewProducer(conn, "test-producer", queueName)
	require.NoError(t, err)

	consumerOpts := rabbitmq.QueueOptions{Durable: true, AutoAck: false}
	consumer, err := rabbitmq.NewConsumer(conn, "test-consumer", queueName, consumerOpts)
	require.NoError(t, err)

	// Start consumer
	messageReceived := false
	go func() {
		consumer.Start(ctx, func(msgCtx context.Context, msg *rabbitmq.Message) {
			messageReceived = true
			msg.Ack()
		})
	}()

	// Wait for consumer to start
	time.Sleep(100 * time.Millisecond)

	// Publish a message
	publishOpts := rabbitmq.PublishOptions{
		Exchange:    "",
		RoutingKey:  queueName,
		ContentType: "text/plain",
		Body:        []byte("shutdown-test"),
	}
	err = producer.Publish(ctx, []byte("shutdown-test"), publishOpts)
	require.NoError(t, err)

	// Wait for message to be processed
	helpers.WaitForCondition(t, func() bool { return messageReceived }, 5*time.Second, "message should be received")

	// Test graceful shutdown
	start := time.Now()

	// Close components in order
	producer.Close()
	conn.Close()

	elapsed := time.Since(start)

	// Shutdown should be quick
	assert.Less(t, elapsed, 2*time.Second, "shutdown should complete quickly")
	assert.True(t, messageReceived, "message should have been processed before shutdown")
}

// TODO: Additional integration tests that require complex setup
func TestAutoReconnectConsumerSide(t *testing.T) {
	t.Skip("TODO: Auto-reconnect test requires more complex container restart handling")
}

func TestPrefetchQoSRespected(t *testing.T) {
	t.Skip("TODO: Prefetch/QoS configuration not exposed in current API")
}
