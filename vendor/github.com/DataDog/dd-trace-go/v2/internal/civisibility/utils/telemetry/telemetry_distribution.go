// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package telemetry

import "github.com/DataDog/dd-trace-go/v2/internal/telemetry"

// EndpointPayloadBytes records the size in bytes of the serialized payload by CI Visibility.
func EndpointPayloadBytes(endpointType EndpointType, value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "endpoint_payload.bytes", removeEmptyStrings([]string{
		string(endpointType),
	})).Submit(value)
}

// EndpointPayloadRequestsMs records the time it takes to send the payload sent to the endpoint in ms by CI Visibility.
func EndpointPayloadRequestsMs(endpointType EndpointType, value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "endpoint_payload.requests_ms", removeEmptyStrings([]string{
		string(endpointType),
	})).Submit(value)
}

// EndpointPayloadEventsCount records the number of events in the payload sent to the endpoint by CI Visibility.
func EndpointPayloadEventsCount(endpointType EndpointType, value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "endpoint_payload.events_count", removeEmptyStrings([]string{
		string(endpointType),
	})).Submit(value)
}

// EndpointEventsSerializationMs records the time it takes to serialize the events in the payload sent to the endpoint in ms by CI Visibility.
func EndpointEventsSerializationMs(endpointType EndpointType, value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "endpoint_payload.events_serialization_ms", removeEmptyStrings([]string{
		string(endpointType),
	})).Submit(value)
}

// GitCommandMs records the time it takes to execute a git command in ms by CI Visibility.
func GitCommandMs(commandType CommandType, value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "git.command_ms", removeEmptyStrings([]string{
		(string)(commandType),
	})).Submit(value)
}

// GitRequestsSearchCommitsMs records the time it takes to get the response of the search commit quest in ms by CI Visibility.
func GitRequestsSearchCommitsMs(responseCompressedType ResponseCompressedType, value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "git_requests.search_commits_ms", removeEmptyStrings([]string{
		(string)(responseCompressedType),
	})).Submit(value)
}

// GitRequestsObjectsPackMs records the time it takes to get the response of the objects pack request in ms by CI Visibility.
func GitRequestsObjectsPackMs(value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "git_requests.objects_pack_ms", nil).Submit(value)
}

// GitRequestsObjectsPackBytes records the sum of the sizes of the object pack files inside a single payload by CI Visibility
func GitRequestsObjectsPackBytes(value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "git_requests.objects_pack_bytes", nil).Submit(value)
}

// GitRequestsObjectsPackFiles records the number of files sent in the object pack payload by CI Visibility.
func GitRequestsObjectsPackFiles(value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "git_requests.objects_pack_files", nil).Submit(value)
}

// GitRequestsSettingsMs records the time it takes to get the response of the settings endpoint request in ms by CI Visibility.
func GitRequestsSettingsMs(value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "git_requests.settings_ms", nil).Submit(value)
}

// ITRSkippableTestsRequestMs records the time it takes to get the response of the itr skippable tests endpoint request in ms by CI Visibility.
func ITRSkippableTestsRequestMs(value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "itr_skippable_tests.request_ms", nil).Submit(value)
}

// ITRSkippableTestsResponseBytes records the number of bytes received by the endpoint. Tagged with a boolean flag set t if response body is compressed.
func ITRSkippableTestsResponseBytes(responseCompressedType ResponseCompressedType, value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "itr_skippable_tests.response_bytes", removeEmptyStrings([]string{
		(string)(responseCompressedType),
	})).Submit(value)
}

// CodeCoverageFiles records the number of files in the code coverage report by CI Visibility.
func CodeCoverageFiles(value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "code_coverage.files", nil).Submit(value)
}

// KnownTestsRequestMs records the time it takes to get the response of the known tests endpoint request in ms by CI Visibility.
func KnownTestsRequestMs(value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "known_tests.request_ms", nil).Submit(value)
}

// KnownTestsResponseBytes records the number of bytes received by the endpoint. Tagged with a boolean flag set to true if response body is compressed.
func KnownTestsResponseBytes(responseCompressedType ResponseCompressedType, value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "known_tests.response_bytes", removeEmptyStrings([]string{
		string(responseCompressedType),
	})).Submit(value)
}

// KnownTestsResponseTests records the number of tests in the response of the known tests endpoint by CI Visibility.
func KnownTestsResponseTests(value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "known_tests.response_tests", nil).Submit(value)
}

// TestManagementTestsRequestMs records the time it takes to get the response of the test management tests endpoint request in ms by CI Visibility.
func TestManagementTestsRequestMs(value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "test_management_tests.request_ms", nil).Submit(value)
}

// TestManagementTestsResponseBytes records the number of bytes received by the endpoint. Tagged with a boolean flag set to true if response body is compressed.
func TestManagementTestsResponseBytes(responseCompressedType ResponseCompressedType, value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "test_management_tests.response_bytes", removeEmptyStrings([]string{
		string(responseCompressedType),
	})).Submit(value)
}

// TestManagementTestsResponseTests records the number of tests in the response of the test management tests endpoint by CI Visibility.
func TestManagementTestsResponseTests(value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "test_management_tests.response_tests", nil).Submit(value)
}

// ImpactedTestsRequestMs records the time it takes to get the response of the impacted tests endpoint request in ms by CI Visibility.
func ImpactedTestsRequestMs(value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "impacted_tests_detection.request_ms", nil).Submit(value)
}

// ImpactedTestsResponseBytes records the number of bytes received by the endpoint. Tagged with a boolean flag set to true if response body is compressed.
func ImpactedTestsResponseBytes(responseCompressedType ResponseCompressedType, value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "impacted_tests_detection.response_bytes", removeEmptyStrings([]string{
		string(responseCompressedType),
	})).Submit(value)
}

// ImpactedTestsResponseFiles records the number of files in the response of the impacted tests endpoint by CI Visibility.
func ImpactedTestsResponseFiles(value float64) {
	telemetry.Distribution(telemetry.NamespaceCIVisibility, "impacted_tests_detection.response_files", nil).Submit(value)
}
