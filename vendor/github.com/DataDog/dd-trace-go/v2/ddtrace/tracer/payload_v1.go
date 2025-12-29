// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package tracer

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/processtags"
	"github.com/tinylib/msgp/msgp"
)

// payloadV1 is a new version of a msgp payload that can be sent to the agent.
// Be aware that payloadV1 follows the same rules and constraints as payloadV04. That is:
//
// payloadV1 is not safe for concurrent use
//
// payloadV1 is meant to be used only once and eventually dismissed with the
// single exception of retrying failed flush attempts.
//
// ⚠️  Warning!
//
// The payloadV1 should not be reused for multiple sets of traces.  Resetting the
// payloadV1 for re-use requires the transport to wait for the HTTP package
// Close the request body before attempting to re-use it again!
type payloadV1 struct {
	// bm keeps track of which fields have been set in the payload
	// bits 1-11 are used for field IDs 1-11. Bit 0 is unused.
	bm bitmap

	// the string ID of the container where the tracer is running
	containerID string // 2

	// the string language name of the tracer
	languageName string // 3

	// the string language version of the tracer
	languageVersion string // 4

	// the string version of the tracer
	tracerVersion string // 5

	// the V4 string UUID representation of a tracer session
	runtimeID string // 6

	// the optional `env` string tag that set with the tracer
	env string // 7

	// the optional string hostname of where the tracer is running
	hostname string // 8

	// the optional string `version` tag for the application set in the tracer
	appVersion string // 9

	// a collection of key to value pairs common in all `chunks`
	attributes map[string]anyValue // 10

	// a list of trace `chunks`
	chunks []traceChunk // 11

	// header specifies the first few bytes in the msgpack stream
	// indicating the type of map (fixmap, map16 or map32)
	// and the number of items contained in the stream.
	header []byte

	// readOff specifies the current read position on the header.
	readOff int

	// writeOff specifies the current read position on the header.
	writeOff int

	// count specifies the number of items (traceChunks) in the stream.
	count uint32

	// fields specifies the number of fields in the payload.
	fields uint32

	// buf holds the sequence of msgpack-encoded items.
	buf []byte

	// reader is used for reading the contents of buf.
	reader *bytes.Reader
}

// newPayloadV1 returns a ready to use payloadV1.
func newPayloadV1() *payloadV1 {
	return &payloadV1{
		attributes: make(map[string]anyValue),
		chunks:     make([]traceChunk, 0),
		readOff:    0,
		writeOff:   0,
	}
}

// push pushes a new item (a traceChunk)into the payload.
func (p *payloadV1) push(t spanList) (stats payloadStats, err error) {
	// We need to hydrate the payload with everything we get from the spans.
	// Conceptually, our `t spanList` corresponds to one `traceChunk`.
	if !p.bm.contains(11) && len(t) > 0 {
		p.bm.set(11)
		atomic.AddUint32(&p.fields, 1)
	}

	// For now, we blindly set the origin, priority, and attributes values for the chunk
	// In the future, attributes should hold values that are shared across all chunks in the payload
	origin, priority, sm, traceID := "", 0, uint32(0), [16]byte{}
	attr := make(map[string]anyValue)
	for _, span := range t {
		if span == nil {
			continue
		}
		// If we haven't seen the service yet, we set it blindly assuming that all the spans created by
		// a service must share the same value.
		if _, ok := attr["service"]; !ok {
			attr["service"] = anyValue{valueType: StringValueType, value: span.Root().service}
		}
		binary.BigEndian.PutUint64(traceID[:8], span.Context().traceID.Upper())
		binary.BigEndian.PutUint64(traceID[8:], span.Context().traceID.Lower())

		if prio, ok := span.Context().SamplingPriority(); ok {
			origin = span.Context().origin // TODO(darccio): are we sure that origin will be shared across all the spans in the chunk?
			priority = prio                // TODO(darccio): the same goes for priority.
			dm := span.context.trace.propagatingTag(keyDecisionMaker)
			if v, err := strconv.ParseInt(dm, 10, 32); err == nil {
				if v < 0 {
					v = -v
				}
				sm = uint32(v)
			} else {
				log.Error("failed to convert decision maker to uint32: %s", err.Error())
			}
		}
	}
	tc := traceChunk{
		spans:             t,
		priority:          int32(priority),
		origin:            origin,
		traceID:           traceID[:],
		samplingMechanism: uint32(sm),
		attributes:        attr,
	}

	// Append process tags to the payload attributes
	// if there are attributes available, set them in our bitmap and increment
	// the number of fields.
	p.setProcessTags()
	if !p.bm.contains(10) && len(p.attributes) > 0 {
		p.bm.set(10)
		atomic.AddUint32(&p.fields, 1)
	}

	p.chunks = append(p.chunks, tc)
	p.recordItem()
	return p.stats(), err
}

// grows the buffer to fit n more bytes. Follows the internal Go standard
// for growing slices (https://github.com/golang/go/blob/master/src/runtime/slice.go#L289)
func (p *payloadV1) grow(n int) {
	cap := cap(p.buf)
	newLen := len(p.buf) + n
	threshold := 256
	for {
		cap += (cap + 3*threshold) >> 2
		if cap >= newLen {
			break
		}
	}
	newBuffer := make([]byte, cap)
	copy(newBuffer, p.buf)
	p.buf = newBuffer
}

func (p *payloadV1) reset() {
	p.updateHeader()
	if p.reader != nil {
		p.reader.Seek(0, 0)
	}
}

func (p *payloadV1) clear() {
	p.bm = 0
	p.buf = p.buf[:0]
	p.reader = nil
	p.header = nil
	p.readOff = 0
	atomic.StoreUint32(&p.fields, 0)
	p.count = 0
}

// recordItem records that a new chunk was added to the payload.
func (p *payloadV1) recordItem() {
	atomic.AddUint32(&p.count, 1)
}

func (p *payloadV1) stats() payloadStats {
	return payloadStats{
		size:      p.size(),
		itemCount: p.itemCount(),
	}
}

func (p *payloadV1) size() int {
	return len(p.buf) + len(p.header) - p.readOff
}

func (p *payloadV1) itemCount() int {
	return int(atomic.LoadUint32(&p.count))
}

func (p *payloadV1) protocol() float64 {
	return traceProtocolV1
}

func (p *payloadV1) updateHeader() {
	if len(p.header) == 0 {
		p.header = make([]byte, 8)
	}
	n := atomic.LoadUint32(&p.fields)
	switch {
	case n <= 15:
		p.header[7] = msgpackMapFix + byte(n)
		p.readOff = 7
	case n <= 1<<16-1:
		binary.BigEndian.PutUint64(p.header, uint64(n)) // writes 2 bytes
		p.header[5] = msgpackMap16
		p.readOff = 5
	default: // n <= 1<<32-1
		binary.BigEndian.PutUint64(p.header, uint64(n)) // writes 4 bytes
		p.header[3] = msgpackMap32
		p.readOff = 3
	}
}

// Set process tags onto the payload attributes
func (p *payloadV1) setProcessTags() {
	if atomic.LoadUint32(&p.count) != 0 {
		return
	}
	pTags := processtags.GlobalTags().String()
	if pTags == "" {
		return
	}
	p.attributes[keyProcessTags] = anyValue{
		valueType: StringValueType,
		value:     pTags,
	}
}

func (p *payloadV1) Close() error {
	p.clear()
	return nil
}

func (p *payloadV1) Write(b []byte) (int, error) {
	p.buf = append(p.buf, b...)
	return len(b), nil
}

// Read implements io.Reader. It reads from the msgpack-encoded stream.
func (p *payloadV1) Read(b []byte) (n int, err error) {
	if len(p.header) == 0 {
		p.header = make([]byte, 8)
		p.updateHeader()
	}
	if p.readOff < len(p.header) {
		// reading header
		n = copy(b, p.header[p.readOff:])
		p.readOff += n
		return n, nil
	}
	if len(p.buf) == 0 {
		p.encode()
	}
	if p.reader == nil {
		p.reader = bytes.NewReader(p.buf)
	}
	return p.reader.Read(b)
}

// encode writes existing payload fields into the buffer in msgp format.
func (p *payloadV1) encode() {
	st := newStringTable()
	p.buf = encodeField(p.buf, p.bm, 2, p.containerID, st)
	p.buf = encodeField(p.buf, p.bm, 3, p.languageName, st)
	p.buf = encodeField(p.buf, p.bm, 4, p.languageVersion, st)
	p.buf = encodeField(p.buf, p.bm, 5, p.tracerVersion, st)
	p.buf = encodeField(p.buf, p.bm, 6, p.runtimeID, st)
	p.buf = encodeField(p.buf, p.bm, 7, p.env, st)
	p.buf = encodeField(p.buf, p.bm, 8, p.hostname, st)
	p.buf = encodeField(p.buf, p.bm, 9, p.appVersion, st)

	p.encodeAttributes(p.bm, 10, p.attributes, st)

	p.encodeTraceChunks(p.bm, 11, p.chunks, st)
}

type fieldValue interface {
	bool | []byte | int32 | int64 | uint32 | uint64 | string
}

// encodeField takes a field of any fieldValue and encodes it into the given buffer
// in msgp format.
func encodeField[F fieldValue](buf []byte, bm bitmap, fieldID uint32, a F, st *stringTable) []byte {
	if !bm.contains(fieldID) {
		return buf
	}
	buf = msgp.AppendUint32(buf, uint32(fieldID)) // msgp key
	switch value := any(a).(type) {
	case string:
		// encode msgp value, either by pulling from string table or writing it directly
		buf = st.serialize(value, buf)
	case bool:
		buf = msgp.AppendBool(buf, value)
	case float64:
		buf = msgp.AppendFloat64(buf, value)
	case int32, int64:
		buf = msgp.AppendInt64(buf, handleIntValue(value))
	case uint32:
		buf = msgp.AppendUint32(buf, value)
	case uint64:
		buf = msgp.AppendUint64(buf, value)
	case []byte:
		buf = msgp.AppendBytes(buf, value)
	case arrayValue:
		buf = msgp.AppendArrayHeader(buf, uint32(len(value)))
		for _, v := range value {
			buf = v.encode(buf, st)
		}
	}
	return buf
}

// encodeAttributes encodes an array associated with fieldID into the buffer in msgp format.
// Each attribute is encoded as three values: the key, value type, and value.
func (p *payloadV1) encodeAttributes(bm bitmap, fieldID int, kv map[string]anyValue, st *stringTable) (bool, error) {
	if !bm.contains(uint32(fieldID)) {
		return false, nil
	}

	p.buf = msgp.AppendUint32(p.buf, uint32(fieldID))        // msgp key
	p.buf = msgp.AppendArrayHeader(p.buf, uint32(len(kv)*3)) // number of item pairs in array

	for k, v := range kv {
		// encode msgp key
		p.buf = st.serialize(k, p.buf)

		// encode value
		p.buf = v.encode(p.buf, st)
	}
	return true, nil
}

// encodeTraceChunks encodes a list of trace chunks associated with fieldID into p.buf in msgp format.
func (p *payloadV1) encodeTraceChunks(bm bitmap, fieldID int, tc []traceChunk, st *stringTable) (bool, error) {
	if len(tc) == 0 || !bm.contains(uint32(fieldID)) {
		return false, nil
	}

	p.buf = msgp.AppendUint32(p.buf, uint32(fieldID))      // msgp key
	p.buf = msgp.AppendArrayHeader(p.buf, uint32(len(tc))) // number of chunks
	for _, chunk := range tc {
		p.buf = msgp.AppendMapHeader(p.buf, 7) // number of fields in chunk

		// priority
		p.buf = encodeField(p.buf, fullSetBitmap, 1, chunk.priority, st)

		// origin
		p.buf = encodeField(p.buf, fullSetBitmap, 2, chunk.origin, st)

		// attributes
		p.encodeAttributes(fullSetBitmap, 3, chunk.attributes, st)

		// spans
		p.encodeSpans(fullSetBitmap, 4, chunk.spans, st)

		// droppedTrace
		p.buf = encodeField(p.buf, fullSetBitmap, 5, chunk.droppedTrace, st)

		// traceID
		p.buf = encodeField(p.buf, fullSetBitmap, 6, chunk.traceID, st)

		// samplingMechanism
		p.buf = encodeField(p.buf, fullSetBitmap, 7, chunk.samplingMechanism, st)
	}

	return true, nil
}

// encodeSpans encodes a list of spans associated with fieldID into p.buf in msgp format.
func (p *payloadV1) encodeSpans(bm bitmap, fieldID int, spans spanList, st *stringTable) (bool, error) {
	if len(spans) == 0 || !bm.contains(uint32(fieldID)) {
		return false, nil
	}

	p.buf = msgp.AppendUint32(p.buf, uint32(fieldID))         // msgp key
	p.buf = msgp.AppendArrayHeader(p.buf, uint32(len(spans))) // number of spans

	for _, span := range spans {
		if span == nil {
			continue
		}
		p.buf = msgp.AppendMapHeader(p.buf, 16) // number of fields in span

		p.buf = encodeField(p.buf, fullSetBitmap, 1, span.service, st)
		p.buf = encodeField(p.buf, fullSetBitmap, 2, span.name, st)
		p.buf = encodeField(p.buf, fullSetBitmap, 3, span.resource, st)
		p.buf = encodeField(p.buf, fullSetBitmap, 4, span.spanID, st)
		p.buf = encodeField(p.buf, fullSetBitmap, 5, span.parentID, st)
		p.buf = encodeField(p.buf, fullSetBitmap, 6, span.start, st)
		p.buf = encodeField(p.buf, fullSetBitmap, 7, span.duration, st)
		p.buf = encodeField(p.buf, fullSetBitmap, 8, span.error != 0, st)

		// span attributes combine the meta (tags), metrics and meta_struct
		attr := map[string]anyValue{}
		for k, v := range span.meta {
			attr[k] = anyValue{
				valueType: StringValueType,
				value:     v,
			}
		}
		for k, v := range span.metrics {
			attr[k] = anyValue{
				valueType: FloatValueType,
				value:     v,
			}
		}
		for k, v := range span.metaStruct {
			av := buildAnyValue(v)
			if av != nil {
				attr[k] = *av
			}
		}
		p.encodeAttributes(fullSetBitmap, 9, attr, st)

		p.buf = encodeField(p.buf, fullSetBitmap, 10, span.spanType, st)
		p.encodeSpanLinks(fullSetBitmap, 11, span.spanLinks, st)
		p.encodeSpanEvents(fullSetBitmap, 12, span.spanEvents, st)

		env := span.meta[ext.Environment]
		p.buf = encodeField(p.buf, fullSetBitmap, 13, env, st)

		version := span.meta[ext.Version]
		p.buf = encodeField(p.buf, fullSetBitmap, 14, version, st)

		component := span.meta[ext.Component]
		p.buf = encodeField(p.buf, fullSetBitmap, 15, component, st)

		spanKind := span.meta[ext.SpanKind]
		p.buf = encodeField(p.buf, fullSetBitmap, 16, getSpanKindValue(spanKind), st)
	}
	return true, nil
}

// translate a span kind string to its uint32 value
func getSpanKindValue(sk string) uint32 {
	switch sk {
	case ext.SpanKindInternal:
		return 1
	case ext.SpanKindServer:
		return 2
	case ext.SpanKindClient:
		return 3
	case ext.SpanKindProducer:
		return 4
	case ext.SpanKindConsumer:
		return 5
	default:
		return 1 // default to internal
	}
}

// translate a span kind uint32 value to its string value
func getSpanKindString(sk uint32) string {
	switch sk {
	case 1:
		return ext.SpanKindInternal
	case 2:
		return ext.SpanKindServer
	case 3:
		return ext.SpanKindClient
	case 4:
		return ext.SpanKindProducer
	case 5:
		return ext.SpanKindConsumer
	default:
		return ext.SpanKindInternal
	}
}

// encodeSpanLinks encodes a list of span links associated with fieldID into p.buf in msgp format.
func (p *payloadV1) encodeSpanLinks(bm bitmap, fieldID int, spanLinks []SpanLink, st *stringTable) (bool, error) {
	if !bm.contains(uint32(fieldID)) {
		return false, nil
	}
	p.buf = msgp.AppendUint32(p.buf, uint32(fieldID))             // msgp key
	p.buf = msgp.AppendArrayHeader(p.buf, uint32(len(spanLinks))) // number of span links

	for _, link := range spanLinks {
		p.buf = msgp.AppendMapHeader(p.buf, 5) // number of fields in span link

		p.buf = encodeField(p.buf, fullSetBitmap, 1, link.TraceID, st)
		p.buf = encodeField(p.buf, fullSetBitmap, 2, link.SpanID, st)

		attr := map[string]anyValue{}
		for k, v := range link.Attributes {
			attr[k] = anyValue{
				valueType: StringValueType,
				value:     v,
			}
		}
		p.encodeAttributes(fullSetBitmap, 3, attr, st)

		p.buf = encodeField(p.buf, fullSetBitmap, 4, link.Tracestate, st)
		p.buf = encodeField(p.buf, fullSetBitmap, 5, link.Flags, st)
	}
	return true, nil
}

// encodeSpanEvents encodes a list of span events associated with fieldID into p.buf in msgp format.
func (p *payloadV1) encodeSpanEvents(bm bitmap, fieldID int, spanEvents []spanEvent, st *stringTable) (bool, error) {
	if !bm.contains(uint32(fieldID)) {
		return false, nil
	}
	p.buf = msgp.AppendUint32(p.buf, uint32(fieldID))              // msgp key
	p.buf = msgp.AppendArrayHeader(p.buf, uint32(len(spanEvents))) // number of span events

	for _, event := range spanEvents {
		p.buf = msgp.AppendMapHeader(p.buf, 3) // number of fields in span event

		p.buf = encodeField(p.buf, fullSetBitmap, 1, event.TimeUnixNano, st)
		p.buf = encodeField(p.buf, fullSetBitmap, 2, event.Name, st)

		attr := map[string]anyValue{}
		for k, v := range event.Attributes {
			switch v.Type {
			case spanEventAttributeTypeString:
				attr[k] = anyValue{
					valueType: StringValueType,
					value:     v.StringValue,
				}
			case spanEventAttributeTypeInt:
				attr[k] = anyValue{
					valueType: IntValueType,
					value:     handleIntValue(v.IntValue),
				}
			case spanEventAttributeTypeDouble:
				attr[k] = anyValue{
					valueType: FloatValueType,
					value:     v.DoubleValue,
				}
			case spanEventAttributeTypeBool:
				attr[k] = anyValue{
					valueType: BoolValueType,
					value:     v.BoolValue,
				}
			case spanEventAttributeTypeArray:
				attr[k] = anyValue{
					valueType: ArrayValueType,
					value:     v.ArrayValue,
				}
			default:
				log.Warn("dropped unsupported span event attribute type %d", v.Type)
			}
		}
		p.encodeAttributes(fullSetBitmap, 3, attr, st)
	}
	return true, nil
}

// Getters for payloadV1 fields
func (p *payloadV1) GetContainerID() string             { return p.containerID }
func (p *payloadV1) GetLanguageName() string            { return p.languageName }
func (p *payloadV1) GetLanguageVersion() string         { return p.languageVersion }
func (p *payloadV1) GetTracerVersion() string           { return p.tracerVersion }
func (p *payloadV1) GetRuntimeID() string               { return p.runtimeID }
func (p *payloadV1) GetEnv() string                     { return p.env }
func (p *payloadV1) GetHostname() string                { return p.hostname }
func (p *payloadV1) GetAppVersion() string              { return p.appVersion }
func (p *payloadV1) GetAttributes() map[string]anyValue { return p.attributes }

func (p *payloadV1) SetContainerID(v string) {
	p.containerID = v
	p.bm.set(2)
	atomic.AddUint32(&p.fields, 1)
}

func (p *payloadV1) SetLanguageName(v string) {
	p.languageName = v
	p.bm.set(3)
	atomic.AddUint32(&p.fields, 1)
}

func (p *payloadV1) SetLanguageVersion(v string) {
	p.languageVersion = v
	p.bm.set(4)
	atomic.AddUint32(&p.fields, 1)
}

func (p *payloadV1) SetTracerVersion(v string) {
	p.tracerVersion = v
	p.bm.set(5)
	atomic.AddUint32(&p.fields, 1)
}

func (p *payloadV1) SetRuntimeID(v string) {
	p.runtimeID = v
	p.bm.set(6)
	atomic.AddUint32(&p.fields, 1)
}

func (p *payloadV1) SetEnv(v string) {
	p.env = v
	p.bm.set(7)
	atomic.AddUint32(&p.fields, 1)
}

func (p *payloadV1) SetHostname(v string) {
	p.hostname = v
	p.bm.set(8)
	atomic.AddUint32(&p.fields, 1)
}

func (p *payloadV1) SetAppVersion(v string) {
	p.appVersion = v
	p.bm.set(9)
	atomic.AddUint32(&p.fields, 1)
}

// decodeBuffer takes the buffer from the payload, decodes it, and populates the fields
// according to the msgpack-encoded byte stream.
func (p *payloadV1) decodeBuffer() ([]byte, error) {
	numFields, o, err := msgp.ReadMapHeaderBytes(p.buf)
	if err != nil {
		return p.buf, err
	}
	p.buf = o
	atomic.StoreUint32(&p.fields, numFields)
	p.header = make([]byte, 8)
	p.updateHeader()

	st := newStringTable()
	fieldCount := 1
	for {
		if len(o) == 0 || err != nil {
			break
		}
		// read msgp field ID
		var idx uint32
		idx, o, err = msgp.ReadUint32Bytes(o)
		if err != nil {
			break
		}

		// handle attributes
		if idx == 10 {
			p.attributes, o, err = decodeAttributes(o, st)
			if err != nil {
				break
			}
			continue
		}

		// handle trace chunks
		if idx == 11 {
			p.chunks, o, err = decodeTraceChunks(o, st)
			if err != nil {
				break
			}
			continue
		}

		// read msgp string value
		var value string
		var ok bool
		value, o, ok = st.read(o)
		if !ok {
			err = errUnableDecodeString
			break
		}

		switch idx {
		case 2:
			p.containerID = value
		case 3:
			p.languageName = value
		case 4:
			p.languageVersion = value
		case 5:
			p.tracerVersion = value
		case 6:
			p.runtimeID = value
		case 7:
			p.env = value
		case 8:
			p.hostname = value
		case 9:
			p.appVersion = value
		default:
			err = fmt.Errorf("unexpected field ID %d", idx)
		}
		fieldCount++
	}
	return o, err
}

// AnyValue is a representation of the `any` value. It can take the following types:
// - uint32
// - bool
// - float64
// - int64
// - uint8
// intValue(5) - 0x405 (4 indicates this is an int AnyType, then 5 is encoded using positive fixed int format)
// stringValue(“a”) - 0x1a161 (1 indicates this is a string, then “a” is encoded using fixstr 0xa161)
// stringValue(2) - 0x102 (1 indicates this is a string, then a positive fixed int of 2 refers the 2nd index of the string table)
type anyValue struct {
	valueType int
	value     interface{}
}

const (
	StringValueType  = iota + 1 // string or uint -- 1
	BoolValueType               // boolean -- 2
	FloatValueType              // float64 -- 3
	IntValueType                // int64 -- 4
	BytesValueType              // []uint8 -- 5
	ArrayValueType              // []AnyValue -- 6
	keyValueListType            // []keyValue -- 7
)

// buildAnyValue builds an anyValue from a given any type.
func buildAnyValue(v any) *anyValue {
	switch v := v.(type) {
	case string:
		return &anyValue{valueType: StringValueType, value: v}
	case bool:
		return &anyValue{valueType: BoolValueType, value: v}
	case float64:
		return &anyValue{valueType: FloatValueType, value: v}
	case int32, int64:
		return &anyValue{valueType: IntValueType, value: handleIntValue(v)}
	case []byte:
		return &anyValue{valueType: BytesValueType, value: v}
	case arrayValue:
		return &anyValue{valueType: ArrayValueType, value: v}
	default:
		return nil
	}
}

func (a anyValue) encode(buf []byte, st *stringTable) []byte {
	buf = msgp.AppendInt32(buf, int32(a.valueType))
	switch a.valueType {
	case StringValueType:
		s := a.value.(string)
		buf = st.serialize(s, buf)
	case BoolValueType:
		buf = msgp.AppendBool(buf, a.value.(bool))
	case FloatValueType:
		buf = msgp.AppendFloat64(buf, a.value.(float64))
	case IntValueType:
		buf = msgp.AppendInt64(buf, a.value.(int64))
	case BytesValueType:
		buf = msgp.AppendBytes(buf, a.value.([]byte))
	case ArrayValueType:
		buf = msgp.AppendArrayHeader(buf, uint32(len(a.value.(arrayValue))))
		for _, v := range a.value.(arrayValue) {
			buf = v.encode(buf, st)
		}
	}
	return buf
}

// translate any int value to int64
func handleIntValue(a any) int64 {
	switch v := a.(type) {
	case int64:
		return v
	case int32:
		return int64(v)
	default:
		// Fallback for other integer types
		return v.(int64)
	}
}

type arrayValue []anyValue

// keeps track of which fields have been set in the payload, with a
// 1 for represented fields and 0 for unset fields.
type bitmap int32

// fullSetBitmap is a bitmap that represents all fields that have been set in the payload.
var fullSetBitmap bitmap = -1

func (b *bitmap) set(bit uint32) {
	if bit >= 32 {
		return
	}
	*b |= 1 << bit
}

func (b bitmap) contains(bit uint32) bool {
	if bit >= 32 {
		return false
	}
	return b&(1<<bit) != 0
}

// an encodable and decodable index of a string in the string table
type index int32

func (i index) encode(buf []byte) []byte {
	return msgp.AppendUint32(buf, uint32(i))
}

func (i *index) decode(buf []byte) ([]byte, error) {
	val, o, err := msgp.ReadUintBytes(buf)
	if err != nil {
		return buf, err
	}
	*i = index(val)
	return o, nil
}

// an encodable and decodable string value
type stringValue string

func (s stringValue) encode(buf []byte) []byte {
	// TODO(hannahkm): add the fixstr representation
	return msgp.AppendString(buf, string(s))
}

func (s *stringValue) decode(buf []byte) ([]byte, error) {
	val, o, err := msgp.ReadStringBytes(buf)
	if err != nil {
		return buf, err
	}
	*s = stringValue(val)
	return o, nil
}

var errUnableDecodeString = errors.New("unable to read string value")

type stringTable struct {
	strings   []stringValue         // list of strings
	indices   map[stringValue]index // map strings to their indices
	nextIndex index                 // last index of the stringTable
}

func newStringTable() *stringTable {
	return &stringTable{
		strings:   []stringValue{""},
		indices:   map[stringValue]index{"": 0},
		nextIndex: 1,
	}
}

// Adds a string to the string table if it does not already exist. Returns the index of the string.
func (s *stringTable) add(str string) (idx index) {
	sv := stringValue(str)
	if _, ok := s.indices[sv]; ok {
		return s.indices[sv]
	}
	s.indices[sv] = s.nextIndex
	s.strings = append(s.strings, sv)
	idx = s.nextIndex
	s.nextIndex += 1
	return
}

// Get returns the index of a string in the string table if it exists. Returns false if the string does not exist.
func (s *stringTable) get(str string) (index, bool) {
	sv := stringValue(str)
	if idx, ok := s.indices[sv]; ok {
		return idx, true
	}
	return -1, false
}

func (st *stringTable) serialize(value string, buf []byte) []byte {
	if idx, ok := st.get(value); ok {
		buf = idx.encode(buf)
	} else {
		s := stringValue(value)
		buf = s.encode(buf)
		st.add(value)
	}
	return buf
}

// Reads a string from a byte slice and returns it from the string table if it exists.
// Returns false if the string does not exist.
func (s *stringTable) read(b []byte) (string, []byte, bool) {
	sType := getStreamingType(b[0])
	if sType == -1 {
		return "", b, false
	}
	// if b is a string
	if sType == 0 {
		var sv stringValue
		o, err := sv.decode(b)
		if err != nil {
			return "", b, false
		}
		str := string(sv)
		s.add(str)
		return str, o, true
	}
	// if b is an index
	var i index
	o, err := i.decode(b)
	if err != nil {
		return "", b, false
	}
	return string(s.strings[i]), o, true
}

// returns 0 if the given byte is a string,
// 1 if it is an int32, and -1 if it is neither.
func getStreamingType(b byte) int {
	switch b {
	// String formats
	case 0xd9, 0xda, 0xdb: // str8, str16, str32
		return 0
	case 0xcc, 0xcd, 0xce: // uint8, uint16, uint32
		return 1
	default:
		// Check for fixstr
		if b&0xe0 == 0xa0 {
			return 0
		}
		// Check for positive fixint
		if b&0x80 == 0 {
			return 1
		}
		return -1
	}
}

// traceChunk represents a list of spans with the same trace ID,
// i.e. a chunk of a trace
type traceChunk struct {
	// the sampling priority of the trace
	priority int32

	// the optional string origin ("lambda", "rum", etc.) of the trace chunk
	origin string

	// a collection of key to value pairs common in all `spans`
	attributes map[string]anyValue

	// a list of spans in this chunk
	spans spanList

	// whether the trace only contains analyzed spans
	// (not required by tracers and set by the agent)
	droppedTrace bool

	// the ID of the trace to which all spans in this chunk belong
	traceID []byte

	// the optional string decision maker (previously span tag _dd.p.dm)
	samplingMechanism uint32
}

// decodeTraceChunks decodes a list of trace chunks from a byte slice.
func decodeTraceChunks(b []byte, st *stringTable) ([]traceChunk, []byte, error) {
	out := []traceChunk{}
	numChunks, o, err := msgp.ReadArrayHeaderBytes(b)
	if err != nil {
		return nil, b, err
	}

	for range numChunks {
		tc := traceChunk{}
		o, err = tc.decode(o, st)
		if err != nil {
			return nil, o, err
		}
		out = append(out, tc)
	}
	return out, o, nil
}

func (tc *traceChunk) decode(b []byte, st *stringTable) ([]byte, error) {
	numFields, o, err := msgp.ReadMapHeaderBytes(b)
	for range numFields {
		if err != nil {
			return b, err
		}
		// read msgp field ID
		var (
			idx uint32
			ok  bool
		)
		idx, o, err = msgp.ReadUint32Bytes(o)
		if err != nil {
			return o, err
		}

		// read msgp string value
		switch idx {
		case 1:
			tc.priority, o, err = msgp.ReadInt32Bytes(o)
		case 2:
			tc.origin, o, ok = st.read(o)
			if !ok {
				err = errUnableDecodeString
			}
		case 3:
			tc.attributes, o, err = decodeAttributes(o, st)
		case 4:
			tc.spans, o, err = decodeSpans(o, st)
		case 5:
			tc.droppedTrace, o, err = msgp.ReadBoolBytes(o)
		case 6:
			tc.traceID, o, err = msgp.ReadBytesBytes(o, nil)
		case 7:
			tc.samplingMechanism, o, err = msgp.ReadUint32Bytes(o)
		default:
			return o, fmt.Errorf("unexpected field ID %d", idx)
		}
	}
	return o, err
}

// decodeSpans decodes a list of spans from a byte slice.
func decodeSpans(b []byte, st *stringTable) (spanList, []byte, error) {
	out := spanList{}
	numSpans, o, err := msgp.ReadArrayHeaderBytes(b)
	if err != nil {
		return nil, b, err
	}
	for range numSpans {
		span := Span{}
		o, err = span.decode(o, st)
		if err != nil {
			return nil, o, err
		}
		out = append(out, &span)
	}
	return out, o, nil
}

// decode reads a span from a byte slice and populates the associated fields in the span.
// This should only be used with decoding v1.0 payloads.
func (span *Span) decode(b []byte, st *stringTable) ([]byte, error) {
	numFields, o, err := msgp.ReadMapHeaderBytes(b)
	for range numFields {
		if err != nil {
			return b, err
		}
		var (
			idx uint32
			ok  bool
		)
		// read msgp field ID
		idx, o, err = msgp.ReadUint32Bytes(o)
		if err != nil {
			return o, err
		}

		// read msgp value
		switch idx {
		case 1:
			span.service, o, ok = st.read(o)
			if !ok {
				err = errUnableDecodeString
			}
		case 2:
			span.name, o, ok = st.read(o)
			if !ok {
				err = errUnableDecodeString
			}
		case 3:
			span.resource, o, ok = st.read(o)
			if !ok {
				err = errUnableDecodeString
			}
		case 4:
			span.spanID, o, err = msgp.ReadUint64Bytes(o)
		case 5:
			span.parentID, o, err = msgp.ReadUint64Bytes(o)
		case 6:
			span.start, o, err = msgp.ReadInt64Bytes(o)
		case 7:
			span.duration, o, err = msgp.ReadInt64Bytes(o)
		case 8:
			var v bool
			v, o, err = msgp.ReadBoolBytes(o)
			if v {
				span.error = 1
			} else {
				span.error = 0
			}
		case 9:
			var attr map[string]anyValue
			attr, o, err = decodeAttributes(o, st)
			for k, v := range attr {
				span.SetTag(k, v.value)
			}
		case 10:
			span.spanType, o, ok = st.read(o)
			if !ok {
				err = errUnableDecodeString
			}
		case 11:
			span.spanLinks, o, err = decodeSpanLinks(o, st)
		case 12:
			span.spanEvents, o, err = decodeSpanEvents(o, st)
		case 13:
			var env string
			env, o, ok = st.read(o)
			if !ok {
				err = errUnableDecodeString
				break
			}
			if env != "" {
				span.SetTag(ext.Environment, env)
			}
		case 14:
			var ver string
			ver, o, ok = st.read(o)
			if !ok {
				err = errUnableDecodeString
				break
			}
			if ver != "" {
				span.SetTag(ext.Version, ver)
			}
		case 15:
			var component string
			component, o, ok = st.read(o)
			if !ok {
				err = errUnableDecodeString
				break
			}
			if component != "" {
				span.SetTag(ext.Component, component)
			}
		case 16:
			var sk uint32
			sk, o, err = msgp.ReadUint32Bytes(o)
			if err != nil {
				return o, err
			}
			span.SetTag(ext.SpanKind, getSpanKindString(sk))
		default:
			return o, fmt.Errorf("unexpected field ID %d", idx)
		}
	}
	return o, err
}

// decodeSpanLinks decodes a list of span links from a byte slice.
func decodeSpanLinks(b []byte, st *stringTable) ([]SpanLink, []byte, error) {
	out := []SpanLink{}
	numLinks, o, err := msgp.ReadArrayHeaderBytes(b)
	if err != nil {
		return nil, b, err
	}
	for range numLinks {
		link := SpanLink{}
		o, err = link.decode(o, st)
		if err != nil {
			return nil, o, err
		}
		out = append(out, link)
	}
	return out, o, nil
}

// decode reads a span link from a byte slice and populates the associated fields in the span link.
// This should only be used with decoding v1.0 payloads.
func (link *SpanLink) decode(b []byte, st *stringTable) ([]byte, error) {
	numFields, o, err := msgp.ReadMapHeaderBytes(b)
	if err != nil {
		return b, err
	}
	for range numFields {
		if err != nil {
			return b, err
		}
		// read msgp field ID
		var idx uint32
		idx, o, err = msgp.ReadUint32Bytes(o)
		if err != nil {
			return o, err
		}

		// read msgp string value
		switch idx {
		case 1:
			link.TraceID, o, err = msgp.ReadUint64Bytes(o)
		case 2:
			link.SpanID, o, err = msgp.ReadUint64Bytes(o)
		case 3:
			var attr map[string]anyValue
			attr, o, err = decodeAttributes(o, st)
			for k, v := range attr {
				if v.valueType != StringValueType {
					return o, fmt.Errorf("unexpected value type: %d", v.valueType)
				}
				link.Attributes[k] = v.value.(string)
			}
		case 4:
			var state string
			var ok bool
			state, o, ok = st.read(o)
			if !ok {
				err = errUnableDecodeString
				break
			}
			link.Tracestate = state
		case 5:
			link.Flags, o, err = msgp.ReadUint32Bytes(o)
		default:
			return o, fmt.Errorf("unexpected field ID %d", idx)
		}
	}
	return o, err
}

// decodeSpanEvents decodes a list of span events from a byte slice.
func decodeSpanEvents(b []byte, st *stringTable) ([]spanEvent, []byte, error) {
	out := []spanEvent{}
	numEvents, o, err := msgp.ReadArrayHeaderBytes(b)
	if err != nil {
		return nil, b, err
	}
	for range numEvents {
		event := spanEvent{}
		o, err = event.decode(o, st)
		if err != nil {
			return nil, o, err
		}
		out = append(out, event)
	}
	return out, o, nil
}

// decode reads a span event from a byte slice and populates the associated fields in the span event.
// This should only be used with decoding v1.0 payloads.
func (event *spanEvent) decode(b []byte, st *stringTable) ([]byte, error) {
	numFields, o, err := msgp.ReadMapHeaderBytes(b)
	if err != nil {
		return b, err
	}
	for range numFields {
		// read msgp field ID
		var idx uint32
		idx, o, err = msgp.ReadUint32Bytes(o)
		if err != nil {
			return o, err
		}
		switch idx {
		case 1:
			event.TimeUnixNano, o, err = msgp.ReadUint64Bytes(o)
			if err != nil {
				return o, err
			}
		case 2:
			var name string
			var ok bool
			name, o, ok = st.read(o)
			if !ok {
				err = errUnableDecodeString
				break
			}
			event.Name = name
		case 3:
			var attr map[string]anyValue
			attr, o, err = decodeAttributes(o, st)
			if err != nil {
				break
			}
			tmp := make(map[string]any)
			for k, v := range attr {
				tmp[k] = v.value
			}
			event.Attributes = toSpanEventAttributeMsg(tmp)
		default:
			return o, fmt.Errorf("unexpected field ID %d", idx)
		}
	}
	return o, err
}

func decodeAnyValue(b []byte, strings *stringTable) (anyValue, []byte, error) {
	vType, o, err := msgp.ReadInt32Bytes(b)
	if err != nil {
		return anyValue{}, b, err
	}
	switch vType {
	case StringValueType:
		var (
			str string
			ok  bool
		)
		str, o, ok = strings.read(o)
		if !ok {
			return anyValue{}, o, errUnableDecodeString
		}
		return anyValue{valueType: StringValueType, value: str}, o, nil
	case BoolValueType:
		var b bool
		b, o, err = msgp.ReadBoolBytes(o)
		if err != nil {
			return anyValue{}, o, err
		}
		return anyValue{valueType: BoolValueType, value: b}, o, nil
	case FloatValueType:
		var f float64
		f, o, err = msgp.ReadFloat64Bytes(o)
		if err != nil {
			return anyValue{}, o, err
		}
		return anyValue{valueType: FloatValueType, value: f}, o, nil
	case IntValueType:
		var i int64
		i, o, err = msgp.ReadInt64Bytes(o)
		if err != nil {
			return anyValue{}, o, err
		}
		intVal := handleIntValue(i)
		return anyValue{valueType: IntValueType, value: intVal}, o, nil
	case BytesValueType:
		var b []byte
		b, o, err = msgp.ReadBytesBytes(o, nil)
		if err != nil {
			return anyValue{}, o, err
		}
		return anyValue{valueType: BytesValueType, value: b}, o, nil
	case ArrayValueType:
		var len uint32
		len, o, err = msgp.ReadArrayHeaderBytes(o)
		if err != nil {
			return anyValue{}, o, err
		}
		arrayValue := make(arrayValue, len/2)
		for i := range len / 2 {
			arrayValue[i], o, err = decodeAnyValue(o, strings)
			if err != nil {
				return anyValue{}, o, err
			}
		}
		return anyValue{valueType: ArrayValueType, value: arrayValue}, o, nil
	case keyValueListType:
		var kv map[string]anyValue
		kv, o, err = decodeKeyValueList(o, strings)
		if err != nil {
			return anyValue{}, o, err
		}
		return anyValue{valueType: keyValueListType, value: kv}, o, nil
	default:
		return anyValue{}, o, fmt.Errorf("invalid value type: %d", vType)
	}
}

// decodeKeyValueList decodes a map of string to anyValue from a byte slice.
func decodeKeyValueList(b []byte, strings *stringTable) (map[string]anyValue, []byte, error) {
	numFields, o, err := msgp.ReadMapHeaderBytes(b)
	if err != nil {
		return nil, b, err
	}

	kv := map[string]anyValue{}
	for i := range numFields {
		var (
			key string
			ok  bool
			av  anyValue
		)
		key, o, ok = strings.read(o)
		if !ok {
			return nil, o, fmt.Errorf("unable to read key of field %d", i)
		}
		av, o, err = decodeAnyValue(o, strings)
		if err != nil {
			return nil, o, err
		}
		kv[key] = av
	}
	return kv, o, nil
}

// decodeAttributes decodes a map of string to anyValue from a byte slice
// Attributes are encoded as an array of key, valueType, and value.
func decodeAttributes(b []byte, strings *stringTable) (map[string]anyValue, []byte, error) {
	n, o, err := msgp.ReadArrayHeaderBytes(b)
	numFields := n / 3
	if err != nil {
		return nil, b, err
	}

	kv := map[string]anyValue{}
	for i := range numFields {
		var (
			key string
			ok  bool
			av  anyValue
		)
		key, o, ok = strings.read(o)
		if !ok {
			return nil, o, fmt.Errorf("unable to read key of field %d", i)
		}
		av, o, err = decodeAnyValue(o, strings)
		if err != nil {
			return nil, o, err
		}
		kv[key] = av
	}
	return kv, o, nil
}
