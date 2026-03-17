# go-mutexasserts

A small library that allows to check if Go mutexes are locked. It can be used to verify invariants/assumptions about [un]locking mutexes within certain functions.

Also, read the ["How to check if a mutex is locked in Go"](https://blog.trailofbits.com/2020/06/09/how-to-check-if-a-mutex-is-locked-in-go/)  blog post on that topic.

## Installation

Use `go get github.com/trailofbits/go-mutexasserts` to install this into your project and you can then import it with `github.com/trailofbits/go-mutexasserts` and use the `mutexasserts` package.

Then, add assertions to your program and compile it with a `debug` tag (e.g., `go build -tags debug`) to enable mutex assertions.

## API

### Assertion functions

The `Assert*` functions print stack trace and exit program when they are called with an object that won't pass assertion.
We decided to use `exit(1)` instead of `panic()` because panics are sometimes (mis?)used for exception handling and we want to generate a fatal error.

**The `mutexasserts.Assert*` functions will only check assertions if program is build with the `debug` tag (so e.g. `go build -tags debug`). Otherwise, those functions will be empty and optimized out by the compiler.**

* `mutexasserts.AssertMutexLocked(m *sync.Mutex)`
* `mutexasserts.AssertRWMutexLocked(m *sync.RWMutex)`
* `mutexasserts.AssertRWMutexRLocked(m *sync.RWMutex)`

### Getting mutex locked state functions

* `mutexasserts.MutexLocked(m *sync.Mutex) bool`
* `mutexasserts.RWMutexLocked(rw *sync.RWMutex) bool`
* `mutexasserts.RWMutexRLocked(rw *sync.RWMutex) bool`
