// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package tracer

import (
	"bytes"
	"time"

	"github.com/tinylib/msgp/msgp"

	"github.com/DataDog/dd-trace-go/v2/internal/civisibility/constants"
	"github.com/DataDog/dd-trace-go/v2/internal/civisibility/utils"
	"github.com/DataDog/dd-trace-go/v2/internal/civisibility/utils/telemetry"
	"github.com/DataDog/dd-trace-go/v2/internal/globalconfig"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/version"
)

// ciVisibilityPayload represents a payload specifically designed for CI Visibility events.
// It uses the generic payload interface and adds methods to handle CI Visibility specific data.
type ciVisibilityPayload struct {
	payload           payload
	serializationTime time.Duration
}

// push adds a new CI Visibility event to the payload buffer.
// It grows the buffer to accommodate the new event, encodes the event in MessagePack format, and updates the event count.
//
// Parameters:
//
//	event - The CI Visibility event to be added to the payload.
//
// Returns:
//
//	An error if encoding the event fails.
func (p *ciVisibilityPayload) push(event *ciVisibilityEvent) (size int, err error) {
	p.payload.grow(event.Msgsize())
	startTime := time.Now()
	defer func() {
		p.serializationTime += time.Since(startTime)
	}()
	if err := msgp.Encode(p.payload, event); err != nil {
		return 0, err
	}
	p.payload.recordItem() // This already calls updateHeader() internally.
	return p.size(), nil
}

// newCiVisibilityPayload creates a new instance of civisibilitypayload.
//
// Returns:
//
//	A pointer to a newly initialized civisibilitypayload instance.
func newCiVisibilityPayload() *ciVisibilityPayload {
	log.Debug("ciVisibilityPayload: creating payload instance")
	return &ciVisibilityPayload{payload: newPayload(traceProtocolV04), serializationTime: 0}
}

// getBuffer retrieves the complete body of the CI Visibility payload, including metadata.
// It reads the current payload buffer, adds metadata, and encodes the entire payload in MessagePack format.
//
// Parameters:
//
//	config - A pointer to the config structure containing environment settings.
//
// Returns:
//
//	A pointer to a bytes.Buffer containing the encoded CI Visibility payload.
//	An error if reading from the buffer or encoding the payload fails.
func (p *ciVisibilityPayload) getBuffer(config *config) (*bytes.Buffer, error) {
	startTime := time.Now()
	log.Debug("ciVisibilityPayload: .getBuffer (count: %d)", p.payload.stats().itemCount)

	// Create a buffer to read the current payload
	payloadBuf := new(bytes.Buffer)
	if _, err := payloadBuf.ReadFrom(p.payload); err != nil {
		return nil, err
	}

	// Create the visibility payload
	visibilityPayload := p.writeEnvelope(config.env, payloadBuf.Bytes())

	// Create a new buffer to encode the visibility payload in MessagePack format
	encodedBuf := new(bytes.Buffer)
	if err := msgp.Encode(encodedBuf, visibilityPayload); err != nil {
		return nil, err
	}

	telemetry.EndpointPayloadEventsCount(telemetry.TestCycleEndpointType, float64(p.payload.stats().itemCount))
	telemetry.EndpointPayloadBytes(telemetry.TestCycleEndpointType, float64(encodedBuf.Len()))
	telemetry.EndpointEventsSerializationMs(telemetry.TestCycleEndpointType, float64((p.serializationTime + time.Since(startTime)).Milliseconds()))
	return encodedBuf, nil
}

func (p *ciVisibilityPayload) writeEnvelope(env string, events []byte) *ciTestCyclePayload {

	/*
			The Payload format in the CI Visibility protocol is like this:
			{
			    "version": 1,
			    "metadata": {
			      "*": {
			        "runtime-id": "...",
			        "language": "...",
			        "library_version": "...",
			        "env": "..."
			      }
			    },
			    "events": [
			      // ...
			    ]
			}

		The event format can be found in the `civisibility_tslv.go` file in the ciVisibilityEvent documentation
	*/

	// Create the metadata map
	allMetadata := map[string]string{
		"language":        "go",
		"runtime-id":      globalconfig.RuntimeID(),
		"library_version": version.Tag,
	}
	if env != "" {
		allMetadata["env"] = env
	}

	// Create the visibility payload
	visibilityPayload := &ciTestCyclePayload{
		Version: 1,
		Metadata: map[string]map[string]string{
			"*": allMetadata,
		},
		Events: events,
	}

	// Check for the test session name and append the tag at the metadata level
	if testSessionName, ok := utils.GetCITags()[constants.TestSessionName]; ok {
		testSessionMap := map[string]string{
			constants.TestSessionName: testSessionName,
		}
		visibilityPayload.Metadata["test_session_end"] = testSessionMap
		visibilityPayload.Metadata["test_module_end"] = testSessionMap
		visibilityPayload.Metadata["test_suite_end"] = testSessionMap
		visibilityPayload.Metadata["test"] = testSessionMap
	}

	return visibilityPayload
}

// stats returns the current stats of the payload.
func (p *ciVisibilityPayload) stats() payloadStats {
	return p.payload.stats()
}

// size returns the payload size in bytes (for backward compatibility).
func (p *ciVisibilityPayload) size() int {
	return p.payload.size()
}

// itemCount returns the number of items available in the stream (for backward compatibility).
func (p *ciVisibilityPayload) itemCount() int {
	return p.payload.itemCount()
}

// protocol returns the protocol version of the payload.
func (p *ciVisibilityPayload) protocol() float64 {
	return p.payload.protocol()
}

// clear empties the payload buffers.
func (p *ciVisibilityPayload) clear() {
	p.payload.clear()
}

// reset sets up the payload to be read a second time.
func (p *ciVisibilityPayload) reset() {
	p.payload.reset()
}

// Read implements io.Reader by reading from the underlying payload.
func (p *ciVisibilityPayload) Read(b []byte) (n int, err error) {
	return p.payload.Read(b)
}

// Close implements io.Closer by closing the underlying payload.
func (p *ciVisibilityPayload) Close() error {
	return p.payload.Close()
}
