[![build](https://github.com/linkdata/deadlock/actions/workflows/go.yml/badge.svg)](https://github.com/linkdata/deadlock/actions/workflows/go.yml)
[![coverage](https://coveralls.io/repos/github/linkdata/deadlock/badge.svg?branch=main)](https://coveralls.io/github/linkdata/deadlock?branch=main)
[![goreport](https://goreportcard.com/badge/github.com/linkdata/deadlock)](https://goreportcard.com/report/github.com/linkdata/deadlock)
[![Docs](https://godoc.org/github.com/linkdata/deadlock?status.svg)](https://godoc.org/github.com/linkdata/deadlock)

# Preface

Based on https://github.com/sasha-s/go-deadlock.

Changes from that package:
* Uses build tags to eliminate all overhead when not enabled
* Tests pass race checker and has full code coverage
* Guards against github.com/petermattis/goid not supporting the current Go version
* Diagnostic output matches `-race` style and uses `runtime.CallersFrames` to get correct line numbers
* Adds `deadlock.Enabled` and `deadlock.Debug` constants
* Adds `Try(R)Lock()` when using go 1.18+
* Drops the dummy implementations for types other than `Mutex` and `RWMutex`

Also uses significantly less memory and CPU:

```
Using this package:
BenchmarkLockSingle-24           2716491               444.1 ns/op            32 B/op          1 allocs/op
BenchmarkLockParallel-24         2132055               483.0 ns/op            43 B/op          1 allocs/op

Using https://github.com/sasha-s/go-deadlock@v0.3.3:
BenchmarkLockSingle-24           1000000              1468 ns/op             593 B/op          3 allocs/op
BenchmarkLockParallel-24          808564              1313 ns/op             593 B/op          3 allocs/op
```

## Installation

```sh
go get github.com/linkdata/deadlock
```

## Usage

The package enables itself when either the `deadlock` or `race` build tag is set, and the
`nodeadlock` build tag is *not* set. The easiest way is to simply use `deadlock.(RW)Mutex` and
run or test your code with the race detector.

```go
import "github.com/linkdata/deadlock"

var mu deadlock.Mutex
mu.Lock()
defer mu.Unlock()

var rw deadlock.RWMutex
rw.RLock()
defer rw.RUnlock()
```

```sh
go run -race .
```

## Deadlocks

Taking the same lock twice in the same goroutine will deadlock:
```go
A.RLock() // or A.Lock()
...
A.Lock() // or A.RLock()
```

Those cases will be reported immediately when they occur. Also, in case we wait for a lock for more than 
`deadlock.Opts.DeadlockTimeout` (30 seconds by default), we also report that as a potential deadlock.
Setting the `DeadlockTimeout` to zero disables this detection.

#### Sample output
```
POTENTIAL DEADLOCK:
goroutine 624 have been trying to lock 0xc0009a20d8 for more than 20ms:
  github.com/linkdata/deadlock.(*DeadlockMutex).Lock()
      /home/user/src/deadlock/deadlock.go:26 +0x113
  github.com/linkdata/deadlock.TestHardDeadlock.func2()
      /home/user/src/deadlock/deadlock_test.go:154 +0x92

goroutine 622 previously locked it from:
  github.com/linkdata/deadlock.(*DeadlockMutex).Lock()
      /home/user/src/deadlock/deadlock.go:26 +0x164
  github.com/linkdata/deadlock.TestHardDeadlock()
      /home/user/src/deadlock/deadlock_test.go:150 +0xe6

goroutine 622 current stack:
goroutine 622 [sleep]:
time.Sleep(0xf4240)
        /usr/local/go/src/runtime/time.go:195 +0x135
github.com/linkdata/deadlock.spinWait(0xc000988340, 0x0?, 0x1)
        /home/user/src/deadlock/deadlock_test.go:25 +0x3e
github.com/linkdata/deadlock.TestHardDeadlock(0xc000988340)
        /home/user/src/deadlock/deadlock_test.go:157 +0x265
testing.tRunner(0xc000988340, 0x6187e8)
        /usr/local/go/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
        /usr/local/go/src/testing/testing.go:1629 +0x806
```

## Inconsistent lock ordering

One of the most common sources of deadlocks is inconsistent lock ordering.
If you have two mutexes A and B, and in one goroutine you have:
```go
A.Lock() // defer A.Unlock() or similar.
...
B.Lock() // defer B.Unlock() or similar.
```
And in another goroutine the order of locks is reversed:
```go
B.Lock() // defer B.Unlock() or similar.
...
A.Lock() // defer A.Unlock() or similar.
```
This does not guarantee a deadlock (maybe the goroutines above can never be running at the same time), but it is bad practice.
Detection is enabled by default, but can be disabled by setting `deadlock.Opts.MaxMapSize` to zero.

#### Sample output
```
POTENTIAL DEADLOCK: Inconsistent locking:
in one goroutine: happened before
  github.com/linkdata/deadlock.(*DeadlockRWMutex).Lock()
      /home/user/src/deadlock/deadlock.go:55 +0xa8
  github.com/linkdata/deadlock.TestLockOrder.func2()
      /home/user/src/deadlock/deadlock_test.go:120 +0x34

happened after
  github.com/linkdata/deadlock.(*DeadlockMutex).Lock()
      /home/user/src/deadlock/deadlock.go:26 +0x11a
  github.com/linkdata/deadlock.TestLockOrder.func2()
      /home/user/src/deadlock/deadlock_test.go:121 +0xa9

in another goroutine: happened before
  github.com/linkdata/deadlock.(*DeadlockMutex).Lock()
      /home/user/src/deadlock/deadlock.go:26 +0xa5
  github.com/linkdata/deadlock.TestLockOrder.func3()
      /home/user/src/deadlock/deadlock_test.go:129 +0x34

happened after
  github.com/linkdata/deadlock.(*DeadlockRWMutex).RLock()
      /home/user/src/deadlock/deadlock.go:74 +0x11a
  github.com/linkdata/deadlock.TestLockOrder.func3()
      /home/user/src/deadlock/deadlock_test.go:130 +0xa6
```

## Debugging constants

It's often helpful to run extra runtime checks during development 
and testing, but you don't want to have that code around in a
production environment. Since these are constants, if the constant is
false, code that depends on it being true gets removed entirely.

We define two:

* `deadlock.Debug` is true if either `race` or `debug` are set.
* `deadlock.Enabled` is true if either `race` or `deadlock` are set and `nodeadlock` is *not* set.

```go
if deadlock.Debug {
    // extra checks or logging go here
}
```

## Configuring

Options are stored in the global variable `deadlock.Opts`. See [Options](https://pkg.go.dev/github.com/linkdata/deadlock#Options).

* `Opts.DeadlockTimeout`: blocking on mutex for longer than DeadlockTimeout is considered a deadlock, ignored if zero
* `Opts.OnPotentialDeadlock`: callback for when a deadlock is detected, or panic if nil
* `Opts.MaxMapSize`: size of happens before // happens after table, disables inconsistent locking order detection if zero
* `Opts.PrintAllCurrentGoroutines`: if true, dump stacktraces of all goroutines when inconsistent locking is detected
* `Opts.LogBuf`: where to write deadlock info/stacktraces, default is `os.Stderr`
