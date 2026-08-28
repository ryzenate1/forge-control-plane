# E2E Test Suite for GamePanel Forge

This directory contains end-to-end tests for the GamePanel Forge control plane, covering the five core scenarios specified in the requirements.

## Overview

The E2E test suite verifies the complete functionality of the GamePanel orchestration system by testing:

1. **Replicated Application** - Application lifecycle with multiple replicas
2. **Health-Gated Deployment** - Deployment with health checks and rollback
3. **Node Failure** - Node failure detection and recovery
4. **GitOps Compose Stack** - Git-backed Compose stack management
5. **Remote Build and Registry** - Remote image building and registry integration

## Architecture

```
forge/api/internal/services/e2e/
├── suite_test.go          # Test infrastructure and setup
├── replicated_app_test.go # Scenario 1: Replicated Application
├── health_gated_deployment_test.go # Scenario 2: Health-Gated Deployment
├── node_failure_test.go   # Scenario 3: Node Failure
├── gitops_compose_test.go # Scenario 4: GitOps Compose Stack
├── remote_build_test.go   # Scenario 5: Remote Build and Registry
├── main_test.go           # Main test runner and reporting
├── report.go              # Test reporting infrastructure
├── Makefile               # Build and test targets
└── README.md              # This file
```

## Prerequisites

### Database
- PostgreSQL database (version 12+ recommended)
- Connection string in `TEST_DATABASE_URL` environment variable
- Example: `postgres://user:password@localhost:5432/database?sslmode=disable`

### Dependencies
- Go 1.20+
- PostgreSQL client libraries
- Docker (for running test containers in future enhancements)

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `TEST_DATABASE_URL` | PostgreSQL connection string | Required |
| `E2E_ENVIRONMENT` | Test environment name | `local` |

## Running Tests

### Run All Tests
```bash
cd gamepanel/forge/api/internal/services/e2e
make test
```

### Run Individual Scenarios
```bash
# Replicated Application
make test-replicated

# Health-Gated Deployment
make test-health

# Node Failure
make test-node

# GitOps Compose Stack
make test-gitops

# Remote Build and Registry
make test-build
```

### Run with Go Test
```bash
# Run all E2E tests
TEST_DATABASE_URL=your_connection_string go test -v -tags=e2e ./...

# Run specific test
TEST_DATABASE_URL=your_connection_string go test -v -tags=e2e -run TestReplicatedApplicationScenario ./...

# Run edge cases
TEST_DATABASE_URL=your_connection_string go test -v -tags=e2e -run TestEdgeCases ./...
```

## Test Scenarios

### 1. Replicated Application

**Description**: Creates an application with two replicas and verifies the complete lifecycle.

**Test Cases**:
- ✅ Create application with two replicas
- ✅ Verify canonical application/service model
- ✅ Verify immutable revision
- ✅ Verify durable deployment operation
- ✅ Scheduler selects compatible nodes
- ✅ Resource reservations are persisted
- ✅ Duplicate placement is prevented
- ✅ Beacon receives versioned commands
- ✅ Two instances run
- ✅ Observations update instance state
- ✅ Frontend displays both instances
- ✅ Scale to three replicas
- ✅ Scale back to one replica
- ✅ Removed instances and reservations are cleaned
- ✅ Test insufficient capacity
- ✅ Test incompatible nodes

**Files**: `replicated_app_test.go`

### 2. Health-Gated Deployment

**Description**: Deploys revision A successfully, then deploys failing revision B and verifies rollback behavior.

**Test Cases**:
- ✅ Deploy revision A successfully
- ✅ Deploy failing revision B
- ✅ B does not become active prematurely
- ✅ Health verification runs
- ✅ Failure is persisted
- ✅ Rollback begins
- ✅ A remains or becomes active
- ✅ B resources are cleaned
- ✅ Operation timeline shows exact failure and rollback
- ✅ Restart the API during rollback (simulated)
- ✅ Restart the worker during rollback (simulated)
- ✅ Final state remains consistent

**Files**: `health_gated_deployment_test.go`

### 3. Node Failure

**Description**: Runs replicas on multiple nodes, disconnects one Beacon, and verifies recovery.

**Test Cases**:
- ✅ Run replicas on multiple nodes
- ✅ Disconnect one Beacon
- ✅ Heartbeat expires, node becomes unavailable
- ✅ Gateway removes unhealthy targets
- ✅ Stateless instance replacement follows policy
- ✅ Reservations are reconciled
- ✅ Duplicate instances are not created after reconnect
- ✅ Stateful/local-storage workloads are not blindly moved
- ✅ Frontend shows recovery reason

**Files**: `node_failure_test.go`

### 4. GitOps Compose Stack

**Description**: Creates a Git-backed Compose stack and verifies GitOps functionality.

**Test Cases**:
- ✅ Create Git-backed Compose stack
- ✅ Source commit is pinned
- ✅ Manifest is normalized
- ✅ Desired commit is persisted
- ✅ Signed webhook triggers one update
- ✅ Duplicate webhook does not duplicate deployment
- ✅ Update preview is accurate
- ✅ Service changes are applied
- ✅ Failing service triggers failure handling
- ✅ Rollback returns to previous revision
- ✅ Local drift is detected
- ✅ Manual reconciliation works
- ✅ Per-service status and logs are visible

**Files**: `gitops_compose_test.go`

### 5. Remote Build and Registry

**Description**: Uses a capable Beacon node as builder and verifies build functionality.

**Test Cases**:
- ✅ Builder capability selection
- ✅ Exact Git commit checkout
- ✅ Dockerfile build
- ✅ Nixpacks build
- ✅ Bounded build context
- ✅ Cancellation
- ✅ Timeout
- ✅ Image digest persistence
- ✅ Private registry authentication
- ✅ Credential masking
- ✅ Push
- ✅ Deploy by immutable digest
- ✅ Worker restart does not duplicate build
- ✅ Temporary data is cleaned

**Files**: `remote_build_test.go`

## Test Infrastructure

### Test Suite Structure

The `TestSuite` struct provides:
- Database connection and store
- Scheduler service
- Replica manager
- Reservation manager
- Event publisher (mocked)
- Logger
- Cleanup function

### Mocking Strategy

The tests use several mocking approaches:

1. **Database**: Real PostgreSQL database with isolated schemas
2. **Publisher**: Mock event publisher that captures events
3. **Git Service**: Mock Git clone service for GitOps tests
4. **Build Service**: Mock build service for remote build tests

### Test Isolation

Each test:
1. Creates a unique database schema
2. Sets up test data in that schema
3. Runs the test
4. Cleans up the schema on completion

This ensures complete isolation between tests.

## Reporting

The test suite generates comprehensive reports in multiple formats:

### JSON Report
Saved to `test-reports/e2e-report.json`

### Markdown Report
Saved to `test-reports/e2e-report.md`

### Console Output
Printed to stdout with summary statistics

### Report Contents
- Test metadata (timestamp, environment, git info)
- Scenario-level results
- Test case-level results
- Summary statistics
- Success rates
- Defects found
- Recommendations

## Validation Results

The tests validate:

### ✅ Passed Validations
- Application lifecycle management
- Replica placement and anti-affinity
- Resource reservation and cleanup
- Health check integration
- Rollback mechanisms
- Node failure detection
- Instance replacement policies
- GitOps workflow
- Webhook processing
- Build orchestration
- Registry integration

### ⚠️ Known Limitations

1. **Real Beacon Integration**: Tests simulate Beacon behavior but don't run actual Beacon nodes
2. **Docker Integration**: Build tests use mocks instead of real Docker builds
3. **Network Partitions**: Node failure tests simulate failures rather than creating real network partitions
4. **Frontend Integration**: Frontend display tests verify data availability rather than actual UI rendering
5. **Real Git Repositories**: GitOps tests use mock Git services

### 🔧 Future Enhancements

1. **Docker Test Containers**: Run PostgreSQL and other services in containers
2. **Real Beacon Nodes**: Spin up actual Beacon nodes for integration testing
3. **Real Git Repositories**: Use temporary Git repositories for GitOps tests
4. **Real Docker Builds**: Execute actual Docker builds in isolated environments
5. **Chaos Engineering**: Introduce real failures (network, process, etc.)
6. **Performance Testing**: Add load and performance tests
7. **Security Testing**: Add security-focused test cases

## Defects Found

During test development, the following potential issues were identified:

### High Priority
- None identified in current implementation

### Medium Priority
- Instance cleanup during scale-down may leave orphaned reservations
- Health gate timeout handling could be more robust
- Webhook deduplication needs careful implementation

### Low Priority
- Build context validation could be more thorough
- Node capability detection could be more dynamic

## Recommendations

### Architecture Improvements
1. **Enhanced Anti-Affinity**: Improve instance spreading across nodes
2. **Better Rollback**: Implement more sophisticated rollback strategies
3. **Resource Cleanup**: Ensure all resources are properly cleaned up
4. **Health Check Improvements**: Add more health check types and configurations

### Testing Improvements
1. **Integration Tests**: Add more integration tests with real components
2. **Chaos Tests**: Add chaos engineering tests
3. **Performance Tests**: Add performance and load tests
4. **Security Tests**: Add security-focused tests

### Operational Improvements
1. **Better Monitoring**: Enhance monitoring of test infrastructure
2. **Test Parallelization**: Allow parallel test execution
3. **Test Retries**: Implement intelligent test retry logic
4. **Test Artifacts**: Collect and store test artifacts

## Development

### Adding New Tests

1. Create a new test file with `_test.go` suffix
2. Use the `TestSuite` for setup and teardown
3. Add test cases following the existing patterns
4. Update the main test runner if needed

### Test Structure Example

```go
func TestNewScenario(t *testing.T) {
    suite := SetupTestSuite(t)
    if suite == nil {
        return
    }
    defer suite.Cleanup()

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()

    // Setup test data
    regionID := suite.CreateTestRegion(ctx, t)
    node1 := suite.CreateTestNode(ctx, t, regionID, 4096, 8192, 102400)

    // Run test cases
    t.Run("Test Case 1", func(t *testing.T) {
        // Test logic here
    })

    t.Run("Test Case 2", func(t *testing.T) {
        // More test logic
    })
}
```

### Mocking Services

Create mock implementations of interfaces for testing:

```go
type mockService struct {
    // Mock state
}

func (m *mockService) Method(ctx context.Context, req Request) (Response, error) {
    // Mock implementation
    return Response{}, nil
}
```

## Troubleshooting

### Database Connection Issues

1. Verify `TEST_DATABASE_URL` is set correctly
2. Check database is running and accessible
3. Verify credentials are correct
4. Check network connectivity

### Test Timeouts

1. Increase timeout in test context
2. Check for slow database queries
3. Verify test data setup is efficient
4. Consider running tests individually

### Schema Conflicts

1. Ensure each test uses a unique schema
2. Check cleanup is working properly
3. Verify database permissions

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add your test cases
4. Ensure all existing tests pass
5. Submit a pull request

## License

This test suite is part of the GamePanel project and is licensed under the same terms.

## Support

For issues or questions:
- Open an issue in the GitHub repository
- Check the documentation
- Review existing test cases for patterns
