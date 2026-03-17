# Locking

This package provides a hybrid approach to lock analysis that combines static lock checking with dynamic deadlock detection. It serves as a drop-in replacement for `sync.Mutex` and `sync.RWMutex` while enabling both compile-time and runtime lock analysis.

## Design Philosophy

The locking package addresses the challenge of comprehensive lock analysis in Go applications by:

1. **Static Analysis Compatibility**: Uses type aliases in the default build to ensure full compatibility with static lock checkers like `checklocks`
2. **Dynamic Deadlock Detection**: Provides runtime deadlock detection through build tags with zero performance overhead in production
3. **Gradual Migration**: Allows incremental replacement of `sync` types throughout the codebase
4. **Lock State Assertions**: Enables runtime verification of lock states for testing and debugging

## Architecture

The package uses build tags to switch between two implementations:

- **Default build** (`!deadlock`): Type aliases to `sync.Mutex/RWMutex` for zero overhead
- **Debug build** (`deadlock`): Wraps [`github.com/linkdata/deadlock`](https://github.com/linkdata/deadlock) for runtime detection

## Usage Examples

### Basic Usage (Drop-in Replacement)

```go
import "github.com/DataDog/dd-trace-go/v2/internal/locking"

type SafeCounter struct {
    mu    locking.Mutex
    count int
}

func (c *SafeCounter) Increment() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.count++
}

// RWMutex usage
type Cache struct {
    mu   locking.RWMutex
    data map[string]interface{}
}

func (c *Cache) Get(key string) interface{} {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.data[key]
}

func (c *Cache) Set(key string, value interface{}) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.data[key] = value
}
```

### Lock State Assertions

The assert package provides runtime verification of lock states using TryLock-based checks. While static analysis tools like [`checklocks`](https://github.com/google/gvisor/blob/master/tools/checklocks/README.md) provide compile-time guarantees through annotations, runtime assertions offer additional guarantees that static analysis cannot provide.

#### Available Assertion Functions

- `MutexLocked(m TryLocker)` - Panics if mutex is NOT locked (works for both Mutex and RWMutex write locks)
- `RWMutexRLocked(m TryRLocker)` - Panics if RWMutex is NOT read-locked (passes for both RLock and Lock)

**Key Distinctions**:

- `MutexLocked()` uses `TryLock()` to verify exclusive lock (Mutex.Lock or RWMutex.Lock)
- `RWMutexRLocked()` uses `TryRLock()` to verify read access is blocked (either RLock or Lock held)

**Implementation**: All assertions use TryLock-based verification:

- TryLock succeeds (returns true) → lock was NOT held → panic (assertion fails)
- TryLock fails (returns false) → lock IS held → no panic (assertion passes)

This approach works consistently without external dependencies in default and debug builds.

**Note**: When building with `deadlock` tag, these assertion functions become no-ops. The go-deadlock library provides comprehensive runtime deadlock detection, and attempting to use TryLock on already-held locks triggers false positives in go-deadlock's recursive locking detection. In deadlock builds, rely on go-deadlock's built-in verification instead of these assertions.

#### Static vs Runtime Analysis

**Static Analysis (checklocks)**:

- Uses annotations like `// +checklocks:mu` to verify lock requirements at compile time
- Cannot detect runtime-dependent lock patterns or complex conditional locking
- May miss violations in dynamically determined code paths
- Excellent for enforcing consistent locking patterns across large codebases

**Runtime Assertions (TryLock-based)**:

- Verify actual lock state during program execution
- Catch violations that static analysis might miss
- Essential for testing complex synchronization scenarios
- Provide definitive verification of lock invariants

#### Runtime Assertion Examples

```go
import "github.com/DataDog/dd-trace-go/v2/internal/locking/assert"

type SafeCounter struct {
    mu    locking.Mutex
    count int
}

// +checklocks:c.mu
func (c *SafeCounter) unsafeIncrement() {
    // Static checker ensures mu is held when this method is called
    // Runtime assertion provides additional guarantee
    assert.MutexLocked(&c.mu)
    c.count++
}

func (c *SafeCounter) Increment() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.unsafeIncrement() // Both static and runtime checks validate this
}

// Complex scenarios where runtime assertions shine
func (c *SafeCounter) ConditionalIncrement(condition bool) {
    if condition {
        c.mu.Lock()
        defer c.mu.Unlock()
    }

    // Static analysis cannot verify this pattern
    // Runtime assertion ensures safety regardless of condition
    if condition {
        assert.MutexLocked(&c.mu)
        c.count++
    }
}

// RWMutex assertions for read/write differentiation
type Cache struct {
    mu   locking.RWMutex
    data map[string]interface{}
}

// +checklocksread:c.mu
func (c *Cache) unsafeGet(key string) interface{} {
    // Verify either read or write lock is held
    assert.RWMutexRLocked(&c.mu) // Passes for both RLock and Lock
    return c.data[key]
}

// +checklocks:c.mu
func (c *Cache) unsafeSet(key string, value interface{}) {
    // Verify write lock is held
    assert.RWMutexLocked(&c.mu) // Only passes for Lock, not RLock
    c.data[key] = value
}
```

#### When to Use Runtime Assertions

Runtime assertions are particularly valuable in:

1. **Test Scenarios**: Verify lock invariants during unit and integration tests
2. **Complex Lock Patterns**: Validate conditional or dynamically determined locking
3. **Debugging**: Identify lock-related issues during development
4. **Critical Sections**: Ensure absolute certainty about lock state in sensitive code
5. **Migration Verification**: Confirm correctness when refactoring locking code

### Testing with Deadlock Detection

Enable deadlock detection during testing:

```shell
# Run tests with deadlock detection
go test -v -timeout=300s -tags=deadlock ./...

# Run specific tracer tests with deadlock detection
go test -v -timeout=300s -tags=debug,deadlock ./ddtrace/tracer

# Run with both debug and deadlock tags for comprehensive testing
go test -v -timeout=300s -tags=debug,deadlock ./internal/...
```

## Implementation Checklist

### Migration Strategy

- [ ] **Phase 1**: Introduce locking package (current phase)
  - [x] Implement build-tag based mutex types
  - [x] Add lock assertion utilities
  - [x] Create comprehensive documentation
  - [ ] Add golangci-lint rules to prevent new `sync.Mutex` usage

- [ ] **Phase 2**: Gradual Migration
  - [ ] Replace `sync.Mutex` with `locking.Mutex` in core packages
  - [ ] Replace `sync.RWMutex` with `locking.RWMutex` in core packages
  - [ ] Update tests to use lock assertions where appropriate
  - [ ] Add deadlock detection to CI pipeline

- [ ] **Phase 3**: Enforcement
  - [ ] Configure golangci-lint to forbid direct `sync.Mutex` imports
  - [ ] Add linting rules to ensure consistent usage
  - [ ] Document exceptions for specific use cases

### Integration with Static Analysis

The package is designed to work seamlessly with static lock checkers like [`checklocks`](https://github.com/google/gvisor/blob/master/tools/checklocks/README.md). The type aliases ensure full compatibility with static analysis tools:

```go
type SafeCounter struct {
    // +checklocks:mu
    count int
    mu    locking.Mutex
}

// +checklocks:c.mu
func (c *SafeCounter) unsafeIncrement() {
    // Static checker will verify mu is held when this is called
    c.count++
}

// +checklocksacquire:c.mu
func (c *SafeCounter) SafeIncrement() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.unsafeIncrement() // Static checker validates this is safe
}

// Combining static and runtime checks for maximum safety
func (c *SafeCounter) IncrementWithFullVerification() {
    c.mu.Lock()
    defer c.mu.Unlock()

    // Static analysis ensures this is safe at compile time
    // Runtime assertion provides additional guarantee
    assert.MutexLocked(&c.mu)
    c.unsafeIncrement()
}
```

#### Checklocks Annotation Compatibility

The locking package supports all [checklocks annotations](https://github.com/google/gvisor/blob/master/tools/checklocks/README.md#annotations):

- `// +checklocks:fieldname` - Field requires associated mutex to be held
- `// +checklocksread:fieldname` - Field requires read or write lock
- `// +checklocksacquire:mutexname` - Function acquires the mutex
- `// +checklocksrelease:mutexname` - Function releases the mutex
- `// +checklocksignore` - Disable checking for specific code sections

This ensures a gradual migration path where static analysis continues to work while adding runtime verification capabilities.

## Testing Scenarios

### Unit Tests

```shell
# Test without deadlock detection (fast)
go test ./internal/locking

# Test with deadlock detection (comprehensive)
go test -tags=deadlock ./internal/locking
```

### Integration Tests

```shell
# Test tracer components with deadlock detection
go test -v -timeout=300s -tags=deadlock ./ddtrace/tracer

# Test all components with maximum detection
go test -v -timeout=300s -tags=debug,deadlock ./...
```

## Build Tag Testing Strategy

### Test Coverage Matrix

| Build Configuration | Test File | Purpose |
|---------------------|-----------|---------|
| Default (`!deadlock && !debug`) | `assert_sync_test.go` | TryLock assertions with sync.Mutex type aliases |
| Debug (`debug && !deadlock`) | `assert_debug_test.go` | TryLock assertions in debug mode |
| Deadlock (`deadlock`) | `assert_test.go` | Assertions with go-deadlock wrapper |
| Debug+Deadlock (`debug && deadlock`) | `assert_debug_deadlock_test.go` | Combined debug and deadlock features |

### CI Integration

The CI pipeline tests with:

- `BUILD_TAGS=debug` - Tests debug-only configuration
- `BUILD_TAGS=debug,deadlock` - Tests combined configuration

### Running Tests Locally

Test all configurations:
```shell
# Default (sync.Mutex type aliases)
go test ./internal/locking/assert

# Debug build
go test -tags=debug ./internal/locking/assert

# Deadlock build
go test -tags=deadlock ./internal/locking/assert

# Debug + Deadlock
go test -tags=debug,deadlock ./internal/locking/assert

# With race detection
go test -race -tags=debug ./internal/locking/assert
go test -race -tags=debug,deadlock ./internal/locking/assert
```

## Performance Considerations

- **Zero Overhead**: In the default build, type aliases ensure no performance penalty
- **Debugging Mode**: Deadlock detection adds runtime overhead, use only in testing
- **Memory Usage**: Default build has identical memory footprint to `sync` types
- **Static Analysis**: Full compatibility with existing static analysis tools

## Dependencies

- [`github.com/linkdata/deadlock`](https://github.com/linkdata/deadlock): Provides runtime deadlock detection

## Troubleshooting

### Common Issues

1. **Build fails with deadlock tag**: Ensure all dependencies are available
2. **Static checker warnings**: Verify type aliases are used correctly
3. **Performance regression**: Check that deadlock detection is not enabled in production builds

### Debug Commands

```shell
# Verify build tags are working correctly
go build -tags=deadlock -v ./internal/locking

# Check for import conflicts
go mod why github.com/linkdata/deadlock

# Validate lock assertions
go test -v -run TestLockAssertions ./internal/locking/assert
```
