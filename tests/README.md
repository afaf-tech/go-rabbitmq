# Go-RabbitMQ Tests

This directory contains unit and integration tests for the go-rabbitmq library.

## Running Tests

### Unit Tests Only (no external dependencies)
```bash
go test ./tests/...
```

### Integration Tests (requires Docker)
```bash
go test -tags=integration ./tests/...
```

### Run All Tests
```bash
# Run unit tests first
go test ./tests/...

# Then run integration tests
go test -tags=integration ./tests/...
```

## Requirements

### Unit Tests
- Go 1.21+
- No external dependencies

### Integration Tests
- Go 1.21+
- Docker daemon running
- github.com/ory/dockertest/v3
- github.com/stretchr/testify

## Test Structure

- `tests/unit/` - Unit tests that don't require RabbitMQ
- `tests/integration/` - Integration tests with real RabbitMQ via Docker
- `tests/integration/helpers/` - Test utilities and Docker helpers

## Environment Variables

No environment variables are required. Integration tests automatically:
- Start RabbitMQ in Docker containers
- Use random ports to avoid conflicts
- Clean up resources after tests complete

## Test Coverage

### Unit Tests
- Configuration validation and defaults
- Context handling and timeouts
- Parameter validation
- Error handling without broker

### Integration Tests
- Full publish/consume workflows
- Auto-reconnection on broker restarts
- Context deadline enforcement
- Message acknowledgment patterns
- Concurrent operations
- Graceful shutdown

## Troubleshooting

If integration tests fail:
1. Ensure Docker daemon is running
2. Check Docker has permission to pull images
3. Verify no port conflicts (tests use random ports)
4. Check available disk space for Docker containers

For verbose test output:
```bash
go test -v ./tests/...
go test -v -tags=integration ./tests/...
```
