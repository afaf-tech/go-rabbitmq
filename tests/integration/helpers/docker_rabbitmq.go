//go:build integration

package helpers

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/require"
)

// StartRabbitMQ starts a RabbitMQ container using dockertest and returns the connection details
func StartRabbitMQ(t *testing.T) (pool *dockertest.Pool, resource *dockertest.Resource, amqpURL string) {
	t.Helper()

	// Create docker pool - try OrbStack first, then standard locations
	var err error
	if _, statErr := os.Stat(os.ExpandEnv("$HOME/.orbstack/run/docker.sock")); statErr == nil {
		t.Logf("Using OrbStack Docker socket")
		pool, err = dockertest.NewPool("unix://" + os.ExpandEnv("$HOME/.orbstack/run/docker.sock"))
	} else {
		t.Logf("Using default Docker socket")
		pool, err = dockertest.NewPool("")
	}
	require.NoError(t, err, "failed to create docker pool")

	// Set timeouts
	pool.MaxWait = 120 * time.Second

	// Start RabbitMQ container
	resource, err = pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "rabbitmq",
		Tag:        "3.13-management",
		Env: []string{
			"RABBITMQ_DEFAULT_USER=rmq",
			"RABBITMQ_DEFAULT_PASS=rmq",
		},
		ExposedPorts: []string{"5672/tcp"},
	}, func(config *docker.HostConfig) {
		// Set AutoRemove to true so that stopped container goes away by itself
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	require.NoError(t, err, "failed to start RabbitMQ container")

	// Get the mapped port
	hostPort := resource.GetPort("5672/tcp")
	amqpURL = fmt.Sprintf("amqp://rmq:rmq@localhost:%s/", hostPort)

	t.Logf("RabbitMQ container started on port %s", hostPort)

	// Wait for RabbitMQ to be ready
	pool.Retry(func() error {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%s", hostPort), 1*time.Second)
		if err != nil {
			t.Logf("Waiting for RabbitMQ to be ready... %v", err)
			return err
		}
		conn.Close()
		return nil
	})

	t.Logf("RabbitMQ is ready at %s", amqpURL)

	// Add cleanup
	t.Cleanup(func() {
		StopRabbitMQ(t, pool, resource)
	})

	return pool, resource, amqpURL
}

// StopRabbitMQ stops and removes the RabbitMQ container
func StopRabbitMQ(t *testing.T, pool *dockertest.Pool, resource *dockertest.Resource) {
	t.Helper()

	if resource != nil {
		err := pool.Purge(resource)
		if err != nil {
			t.Logf("Failed to purge RabbitMQ container: %v", err)
		} else {
			t.Logf("RabbitMQ container stopped and removed")
		}
	}
}

// RestartRabbitMQ restarts the RabbitMQ container
func RestartRabbitMQ(t *testing.T, pool *dockertest.Pool, resource *dockertest.Resource) {
	t.Helper()

	// Stop the container
	err := pool.Client.StopContainer(resource.Container.ID, 10)
	require.NoError(t, err, "failed to stop RabbitMQ container")

	t.Logf("RabbitMQ container stopped")

	// Start the container again
	err = pool.Client.StartContainer(resource.Container.ID, nil)
	require.NoError(t, err, "failed to start RabbitMQ container")

	t.Logf("RabbitMQ container restarted")

	// Wait for RabbitMQ to be ready again
	hostPort := resource.GetPort("5672/tcp")
	pool.Retry(func() error {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%s", hostPort), 1*time.Second)
		if err != nil {
			t.Logf("Waiting for RabbitMQ to be ready after restart... %v", err)
			return err
		}
		conn.Close()
		return nil
	})

	t.Logf("RabbitMQ is ready again after restart")
}
