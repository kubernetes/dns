// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

//go:generate go run github.com/tinylib/msgp -o=stacktrace_msgp.go -tests=false
//go:generate env GOWORK=off go run ../../scripts/gencontribs/main.go ../.. contribs_generated.go

package stacktrace

import (
	"errors"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/DataDog/dd-trace-go/v2/internal/env"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

var (
	enabled              = true
	defaultTopFrameDepth = 8
	defaultMaxDepth      = 32

	// internalPackagesPrefixes is the list of prefixes for internal packages that should be hidden in the stack trace
	internalSymbolPrefixes = []string{
		"github.com/DataDog/dd-trace-go/v2",
		"gopkg.in/DataDog/dd-trace-go.v1",
		"github.com/DataDog/go-libddwaf",
		"github.com/DataDog/datadog-agent",
		"github.com/datadog/orchestrion",
		"github.com/DataDog/orchestrion",
	}

	// knownThirdPartyLibraries contains third-party library patterns for stack frame classification.
	// This list is automatically generated from contrib/ directory structure at build time,
	// with some fallback patterns for libraries not covered by contrib integrations.
	knownThirdPartyLibraries = generatedThirdPartyLibraries()

	// thirdPartyTrie provides fast O(m) prefix matching for third-party libraries
	// where m is the length of the string being checked, rather than O(n) linear search
	// where n is the number of prefixes (765+ libraries). This provides significant
	// performance improvements especially for stack trace generation.
	thirdPartyTrie *segmentPrefixTrie

	// internalPrefixTrie provides fast prefix matching for internal package prefixes
	internalPrefixTrie *segmentPrefixTrie
)

// Redaction-specific frame types for secure logging
type frameType string

const (
	defaultCallerSkip = 4

	envStackTraceDepth   = "DD_APPSEC_MAX_STACK_TRACE_DEPTH"
	envStackTraceEnabled = "DD_APPSEC_STACK_TRACE_ENABLE"

	frameTypeDatadog    frameType = "datadog"
	frameTypeRuntime    frameType = "runtime"
	frameTypeThirdParty frameType = "third_party"
	frameTypeCustomer   frameType = "customer"

	redactedPlaceholder = "REDACTED"
)

func init() {
	if env := env.Get(envStackTraceEnabled); env != "" {
		if e, err := strconv.ParseBool(env); err == nil {
			enabled = e
		} else {
			log.Error("Failed to parse %s env var as boolean: (using default value: %t) %v", envStackTraceEnabled, enabled, err.Error())
		}
	}

	if env := env.Get(envStackTraceDepth); env != "" {
		if !enabled {
			log.Warn("Ignoring %s because stacktrace generation is disable", envStackTraceDepth)
			return
		}

		if depth, err := strconv.Atoi(env); err == nil {
			defaultMaxDepth = depth
		} else {
			if depth <= 0 {
				err = errors.New("value is not a strictly positive integer")
			}
			log.Error("Failed to parse %s env var as a positive integer: (using default value: %d) %v", envStackTraceDepth, defaultMaxDepth, err.Error())
		}
	}

	defaultTopFrameDepth = defaultMaxDepth / 4

	thirdPartyTrie = newSegmentPrefixTrie()
	thirdPartyTrie.InsertAll(slices.Concat(knownThirdPartyLibraries, []string{"golang.org/"}))

	internalPrefixTrie = newSegmentPrefixTrie()
	internalPrefixTrie.InsertAll(internalSymbolPrefixes)
}

// Enabled returns whether stacktrace should be collected
func Enabled() bool {
	return enabled
}

type (
	// StackTrace is intended to be sent over the span tag `_dd.stack`, the first frame is the current frame
	StackTrace []StackFrame

	// StackFrame represents a single frame in the stack trace
	StackFrame struct {
		Text      string `msg:"text,omitempty"`       // Text version of the stackframe as a string
		File      string `msg:"file,omitempty"`       // File name where the code line is
		Namespace string `msg:"namespace,omitempty"`  // Namespace is the fully qualified name of the package where the code is
		ClassName string `msg:"class_name,omitempty"` // ClassName is the fully qualified name of the class where the line of code is
		Function  string `msg:"function,omitempty"`   // Function is the fully qualified name of the function where the line of code is
		Index     uint32 `msg:"id"`                   // Index of the frame (0 = top of the stack)
		Line      uint32 `msg:"line,omitempty"`       // Line number in the context of the file where the code is
		Column    uint32 `msg:"column,omitempty"`     // Column where the code ran is
	}

	// RawStackTrace represents captured program counters without symbolication.
	// This allows for fast capture with deferred processing - symbolication,
	// skipping, and redaction can be performed later when needed.
	RawStackTrace struct {
		PCs []uintptr `msg:"-"`
	}

	symbol struct {
		Package  string
		Receiver string
		Function string
	}
)

// queue is a simple circular buffer for storing the most recent frames.
// It is NOT thread-safe and is intended for single-goroutine use only.
type queue[T any] struct {
	data       []T
	head, tail int
	size, cap  int
}

func newQueue[T any](capacity int) *queue[T] {
	return &queue[T]{
		data: make([]T, capacity),
		cap:  capacity,
	}
}

func (q *queue[T]) Length() int {
	return q.size
}

func (q *queue[T]) Add(item T) {
	if q.size == q.cap {
		// Overwrite oldest
		q.data[q.tail] = item
		q.tail = (q.tail + 1) % q.cap
		q.head = q.tail
	} else {
		q.data[q.head] = item
		q.head = (q.head + 1) % q.cap
		q.size++
	}
}

func (q *queue[T]) Remove() T {
	if q.size == 0 {
		var zero T
		return zero
	}
	item := q.data[q.tail]
	q.tail = (q.tail + 1) % q.cap
	q.size--
	return item
}

var symbolRegex = regexp.MustCompile(`^(([^(]+/)?([^(/.]+)?)(\.\(([^/)]+)\))?\.([^/()]+)$`)

// parseSymbol parses a symbol name into its package, receiver and function
// ex: github.com/DataDog/dd-trace-go/v2/internal/stacktrace.(*Event).NewException
// -> package: github.com/DataDog/dd-trace-go/v2/internal/stacktrace
// -> receiver: *Event
// -> function: NewException
func parseSymbol(name string) symbol {
	matches := symbolRegex.FindStringSubmatch(name)
	if len(matches) != 7 {
		log.Error("Failed to parse symbol for stacktrace: %s", name)
		return symbol{
			Package:  "",
			Receiver: "",
			Function: "",
		}
	}

	return symbol{
		Package:  matches[1],
		Receiver: matches[5],
		Function: matches[6],
	}
}

// Capture create a new stack trace from the current call stack
func Capture() StackTrace {
	return SkipAndCapture(defaultCallerSkip)
}

// SkipAndCapture creates a new stack trace from the current call stack, skipping the first `skip` frames
func SkipAndCapture(skip int) StackTrace {
	return iterator(skip, defaultMaxDepth, frameOptions{
		skipInternalFrames:      true,
		redactCustomerFrames:    false,
		internalPackagePrefixes: internalSymbolPrefixes,
	}).capture()
}

// CaptureRaw captures only program counters without symbolication.
// This is significantly faster than full capture as it avoids runtime.CallersFrames
// and symbol parsing. The skip parameter determines how many frames to skip from
// the top of the stack (similar to runtime.Callers).
func CaptureRaw(skip int) RawStackTrace {
	pcs := make([]uintptr, defaultMaxDepth)
	n := runtime.Callers(skip, pcs)
	return RawStackTrace{
		PCs: pcs[:n],
	}
}

// CaptureWithRedaction creates a stack trace with customer code redaction but keeps internal Datadog frames
// This is designed for telemetry logging where we want to see internal frames for debugging
// but need to redact customer code for security
func CaptureWithRedaction(skip int) StackTrace {
	return iterator(skip+1, defaultMaxDepth, frameOptions{
		skipInternalFrames:      false, // Keep DD internal frames
		redactCustomerFrames:    true,  // Redact customer code
		internalPackagePrefixes: internalSymbolPrefixes,
	}).capture()
}

// Symbolicate converts raw PCs to a full StackTrace with symbolication,
// applying the default skipping and redaction rules (skips internal frames,
// no customer code redaction).
func (r RawStackTrace) Symbolicate() StackTrace {
	if len(r.PCs) == 0 {
		return nil
	}

	return iteratorFromRaw(r.PCs, frameOptions{
		skipInternalFrames:      true,
		redactCustomerFrames:    false,
		internalPackagePrefixes: internalSymbolPrefixes,
	}).capture()
}

// SymbolicateWithRedaction converts raw PCs to a StackTrace with
// customer code redaction (for telemetry logging). This keeps internal
// Datadog frames but redacts customer code for security.
func (r RawStackTrace) SymbolicateWithRedaction() StackTrace {
	if len(r.PCs) == 0 {
		return nil
	}

	return iteratorFromRaw(r.PCs, frameOptions{
		skipInternalFrames:      false, // Keep DD internal frames
		redactCustomerFrames:    true,  // Redact customer code
		internalPackagePrefixes: internalSymbolPrefixes,
	}).capture()
}

// capture extracts frames from an iterator using the same algorithm as capture
func (iter *framesIterator) capture() StackTrace {
	stack := make([]StackFrame, iter.cacheSize)
	nbStoredFrames := 0
	topFramesQueue := newQueue[StackFrame](defaultTopFrameDepth)

	// We have to make sure we don't store more than maxDepth frames
	// if there is more than maxDepth frames, we get X frames from the bottom of the stack and Y from the top
	for frame, ok := iter.Next(); ok; frame, ok = iter.Next() {
		// we reach the top frames: start to use the queue
		if nbStoredFrames >= defaultMaxDepth-defaultTopFrameDepth {
			topFramesQueue.Add(frame)
			// queue is full, remove the oldest frame
			if topFramesQueue.Length() > defaultTopFrameDepth {
				topFramesQueue.Remove()
			}
			continue
		}

		// Bottom frames: directly store them in the stack
		stack[nbStoredFrames] = frame
		nbStoredFrames++
	}

	// Stitch the top frames to the stack
	for topFramesQueue.Length() > 0 {
		stack[nbStoredFrames] = topFramesQueue.Remove()
		nbStoredFrames++
	}

	return stack[:nbStoredFrames]
}

// frameOptions configures iterator behavior for frame processing
type frameOptions struct {
	internalPackagePrefixes []string // Prefixes for internal packages
	skipInternalFrames      bool     // Whether to skip internal DD frames
	redactCustomerFrames    bool     // Whether to redact customer code frames
}

// framesIterator is an iterator over the frames of a call stack
// It skips internal packages and caches the frames to avoid multiple calls to runtime.Callers
// It also skips the first `skip` frames and can redact customer code for secure logging
//
// IMPORTANT: This iterator is NOT thread-safe and should only be used within a single goroutine.
// Each call to Capture/SkipAndCapture/CaptureWithRedaction creates a new iterator instance.
type framesIterator struct {
	frames     *queue[runtime.Frame]
	frameOpts  frameOptions
	rawPCs     []uintptr
	cache      []uintptr
	cacheSize  int
	cacheDepth int
	currDepth  int
	useRawPCs  bool
}

func iterator(skip, cacheSize int, opts frameOptions) *framesIterator {
	return &framesIterator{
		frameOpts:  opts,
		frames:     newQueue[runtime.Frame](cacheSize + 4),
		cache:      make([]uintptr, cacheSize),
		cacheSize:  cacheSize,
		cacheDepth: skip,
		currDepth:  0,
	}
}

// iteratorFromRaw creates an iterator from pre-captured PCs for deferred symbolication
func iteratorFromRaw(pcs []uintptr, opts frameOptions) *framesIterator {
	cacheSize := min(len(pcs), defaultMaxDepth)

	return &framesIterator{
		frameOpts:  opts,
		frames:     newQueue[runtime.Frame](cacheSize + 4),
		cache:      make([]uintptr, cacheSize),
		cacheSize:  cacheSize,
		cacheDepth: 0,
		useRawPCs:  true,
		rawPCs:     pcs,
		currDepth:  0,
	}
}

// prepareNextBatch returns the next batch of program counters to symbolicate.
// Returns nil slice if no more frames are available.
func (it *framesIterator) prepareNextBatch() []uintptr {
	if it.useRawPCs {
		// Use pre-captured PCs for deferred symbolication.
		remaining := len(it.rawPCs) - it.cacheDepth
		if remaining == 0 {
			return nil
		}

		// Process a batch of PCs up to cacheSize.
		end := min(it.cacheDepth+it.cacheSize, len(it.rawPCs))
		pcs := it.rawPCs[it.cacheDepth:end]
		it.cacheDepth = end
		return pcs
	}

	// Live mode: call runtime.Callers.
	n := runtime.Callers(it.cacheDepth, it.cache)
	if n == 0 {
		return nil
	}

	it.cacheDepth += n
	return it.cache[:n]
}

// symbolicateFrames converts program counters to runtime.Frame objects
// and adds them to the frames queue.
func (it *framesIterator) symbolicateFrames(pcs []uintptr) {
	frames := runtime.CallersFrames(pcs)
	for {
		frame, more := frames.Next()
		it.frames.Add(frame)
		if !more {
			break
		}
	}
}

// next returns the next runtime.Frame in the call stack, filling the cache if needed
func (it *framesIterator) next() (runtime.Frame, bool) {
	if it.frames.Length() == 0 {
		pcs := it.prepareNextBatch()
		if pcs == nil {
			return runtime.Frame{}, false
		}
		it.symbolicateFrames(pcs)
	}

	it.currDepth++
	return it.frames.Remove(), true
}

// Next returns the next StackFrame in the call stack, skipping internal packages and refurbishing the cache if needed
func (it *framesIterator) Next() (StackFrame, bool) {
	for {
		frame, ok := it.next()
		if !ok {
			return StackFrame{}, false
		}

		if it.skipFrame(frame) {
			continue
		}

		var (
			parsedSymbol = parseSymbol(frame.Function)
			shouldRedact = it.shouldRedactSymbol(parsedSymbol)
			stackFrame   = StackFrame{
				Index:     uint32(it.currDepth - 1),
				Text:      "",
				File:      frame.File,
				Line:      uint32(frame.Line),
				Column:    0, // No column given by the runtime
				Namespace: parsedSymbol.Package,
				ClassName: parsedSymbol.Receiver,
				Function:  parsedSymbol.Function,
			}
		)
		if shouldRedact {
			stackFrame.Function = redactedPlaceholder
			stackFrame.File = redactedPlaceholder
			stackFrame.Line = 0
			stackFrame.Namespace = ""
			stackFrame.ClassName = ""
		}

		return stackFrame, true
	}
}

func (it *framesIterator) skipFrame(frame runtime.Frame) bool {
	if frame.File == "<generated>" {
		return true
	}

	// Always skip internal stacktrace implementation methods (but not test functions)
	funcName := frame.Function
	if strings.HasPrefix(funcName,
		"github.com/DataDog/dd-trace-go/v2/internal/stacktrace.(*framesIterator).") ||
		strings.Contains(funcName,
			"github.com/DataDog/dd-trace-go/v2/internal/stacktrace.iterator") {
		return true
	}

	if it.frameOpts.skipInternalFrames {
		if internalPrefixTrie.HasPrefix(frame.Function) {
			return true
		}
	}

	return false
}

func (it *framesIterator) shouldRedactSymbol(sym symbol) bool {
	if !it.frameOpts.redactCustomerFrames {
		return false
	}
	return classifySymbol(sym, it.frameOpts.internalPackagePrefixes) == frameTypeCustomer
}

func classifySymbol(sym symbol, internalPrefixes []string) frameType {
	pkg := sym.Package

	for _, prefix := range internalPrefixes {
		if strings.HasPrefix(pkg, prefix) {
			return frameTypeDatadog
		}
	}

	if isStandardLibraryPackage(pkg) {
		return frameTypeRuntime
	}

	if isKnownThirdPartyLibrary(pkg) {
		return frameTypeThirdParty
	}

	return frameTypeCustomer
}

// Format converts a StackTrace to a string representation
func Format(stack StackTrace) string {
	if len(stack) == 0 {
		return ""
	}

	var result []byte
	for i, frame := range stack {
		if i > 0 {
			result = append(result, '\n')
		}

		// Use full function name (namespace + class + function)
		function := frame.Function
		if frame.Namespace != "" {
			if frame.ClassName != "" {
				function = frame.Namespace + ".(" + frame.ClassName + ")." + frame.Function
			} else {
				function = frame.Namespace + "." + frame.Function
			}
		}

		result = append(result, function...)
		result = append(result, '\n', '\t')
		result = append(result, frame.File...)
		result = append(result, ':')
		result = append(result, strconv.Itoa(int(frame.Line))...)
	}

	return string(result)
}

// isKnownThirdPartyLibrary checks if a package is a known third-party library
func isKnownThirdPartyLibrary(pkg string) bool {
	return thirdPartyTrie.HasPrefix(pkg)
}

// isStandardLibraryPackage checks if a package is from Go's standard library
func isStandardLibraryPackage(pkg string) bool {
	// Handle test packages (e.g., "strconv.test", "net/http.test")
	// When running `go test strconv`, Go creates a test binary with package name "strconv.test"
	// Strip the .test suffix if present to check if the remaining part is a stdlib package
	pkg = strings.TrimSuffix(pkg, ".test")

	// Special case: main package is user code, not stdlib
	if pkg == "main" {
		return false
	}

	// Standard library detection: no dot in the first path element
	// Mirrors go/build's IsStandardImportPath.
	// For standard library imports, the first element doesn't contain a dot.
	// See: https://github.com/golang/go/blob/861c90c907db1129dcd1540eecd3c66b6309db7a/src/cmd/go/internal/search/search.go#L529
	// Examples:
	//   "fmt" -> first element "fmt" (no dot) -> standard library
	//   "net/http" -> first element "net" (no dot) -> standard library
	//   "github.com/user/pkg" -> first element "github.com" (has dot) -> NOT standard library
	slash := strings.IndexByte(pkg, '/')
	if slash < 0 {
		// single-element path like "fmt", "os", "runtime"
		return !strings.Contains(pkg, ".")
	}
	// multi-element path like "net/http", "encoding/json", or "github.com/user/pkg"
	first := pkg[:slash]
	return !strings.Contains(first, ".")
}
