// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package telemetry

import (
	"github.com/DataDog/dd-trace-go/v2/internal/civisibility/constants"
	"github.com/DataDog/dd-trace-go/v2/internal/env"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
)

func getTestingFramework(testingFramework string) TestingFramework {
	telemetryFramework := UnknownFramework
	if testingFramework == "golang.org/pkg/testing" {
		telemetryFramework = GoTestingFramework
	}
	return telemetryFramework
}

func GetErrorTypeFromStatusCode(statusCode int) ErrorType {
	switch statusCode {
	case 0:
		return NetworkErrorType
	case 400:
		return StatusCode400ErrorType
	case 401:
		return StatusCode401ErrorType
	case 403:
		return StatusCode403ErrorType
	case 404:
		return StatusCode404ErrorType
	case 408:
		return StatusCode408ErrorType
	case 429:
		return StatusCode429ErrorType
	default:
		if statusCode >= 500 && statusCode < 600 {
			return StatusCode5xxErrorType
		} else if statusCode >= 400 && statusCode < 500 {
			return StatusCode4xxErrorType
		}
		return StatusCodeErrorType
	}
}

func getProviderTestSessionTypeFromProviderString(provider string) TestSessionType {
	switch provider {
	case "appveyor":
		return AppVeyorTestSessionType
	case "azurepipelines":
		return AzurePipelinesTestSessionType
	case "bitbucket":
		return BitbucketTestSessionType
	case "bitrise":
		return BitRiseTestSessionType
	case "buildkite":
		return BuildKiteTestSessionType
	case "circleci":
		return CircleCiTestSessionType
	case "codefresh":
		return CodeFreshTestSessionType
	case "github":
		return GithubActionsTestSessionType
	case "gitlab":
		return GitlabTestSessionType
	case "jenkins":
		return JenkinsTestSessionType
	case "teamcity":
		return TeamcityTestSessionType
	case "travisci":
		return TravisCiTestSessionType
	case "buddy":
		return BuddyCiTestSessionType
	case "awscodepipeline":
		return AwsCodePipelineSessionType
	default:
		return UnsupportedTestSessionType
	}
}

func TestSession(providerName string) {
	var tags []string
	tags = append(tags, getProviderTestSessionTypeFromProviderString(providerName)...)
	if env.Get(constants.CIVisibilityAutoInstrumentationProviderEnvironmentVariable) != "" {
		tags = append(tags, IsAutoInstrumentationTestSessionType...)
	}
	telemetry.Count(telemetry.NamespaceCIVisibility, "test_session", removeEmptyStrings(tags)).Submit(1.0)
}

// EventCreated the number of events created by CI Visibility
func EventCreated(testingFramework string, eventType TestingEventType) {
	tags := []string{string(getTestingFramework(testingFramework))}
	tags = append(tags, eventType...)
	telemetry.Count(telemetry.NamespaceCIVisibility, "event_created", removeEmptyStrings(tags)).Submit(1.0)
}

// EventFinished the number of events finished by CI Visibility
func EventFinished(testingFramework string, eventType TestingEventType) {
	tags := []string{string(getTestingFramework(testingFramework))}
	tags = append(tags, eventType...)
	telemetry.Count(telemetry.NamespaceCIVisibility, "event_finished", removeEmptyStrings(tags)).Submit(1.0)
}

// CodeCoverageStarted the number of code coverage start calls by CI Visibility
func CodeCoverageStarted(testingFramework string, coverageLibraryType CoverageLibraryType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "code_coverage_started", removeEmptyStrings([]string{
		string(getTestingFramework(testingFramework)),
		string(coverageLibraryType),
	})).Submit(1.0)
}

// CodeCoverageFinished the number of code coverage finished calls by CI Visibility
func CodeCoverageFinished(testingFramework string, coverageLibraryType CoverageLibraryType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "code_coverage_finished", removeEmptyStrings([]string{
		string(getTestingFramework(testingFramework)),
		string(coverageLibraryType),
	})).Submit(1.0)
}

// EventsEnqueueForSerialization the number of events enqueued for serialization by CI Visibility
func EventsEnqueueForSerialization() {
	telemetry.Count(telemetry.NamespaceCIVisibility, "events_enqueued_for_serialization", nil).Submit(1.0)
}

// EndpointPayloadRequests the number of requests sent to the endpoint, regardless of success, tagged by endpoint type
func EndpointPayloadRequests(endpointType EndpointType, requestCompressedType RequestCompressedType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "endpoint_payload.requests", removeEmptyStrings([]string{
		string(endpointType),
		string(requestCompressedType),
	})).Submit(1.0)
}

// EndpointPayloadRequestsErrors the number of requests sent to the endpoint that errored, tagget by the error type and endpoint type and status code
func EndpointPayloadRequestsErrors(endpointType EndpointType, errorType ErrorType) {
	tags := []string{string(endpointType)}
	tags = append(tags, errorType...)
	telemetry.Count(telemetry.NamespaceCIVisibility, "endpoint_payload.requests_errors", removeEmptyStrings(tags)).Submit(1.0)
}

// EndpointPayloadDropped the number of payloads dropped after all retries by CI Visibility
func EndpointPayloadDropped(endpointType EndpointType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "endpoint_payload.dropped", removeEmptyStrings([]string{
		string(endpointType),
	})).Submit(1.0)
}

// GitCommand the number of git commands executed by CI Visibility
func GitCommand(commandType CommandType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "git.command", removeEmptyStrings([]string{
		string(commandType),
	})).Submit(1.0)
}

// GitCommandErrors the number of git command that errored by CI Visibility
func GitCommandErrors(commandType CommandType, exitCode CommandExitCodeType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "git.command_errors", removeEmptyStrings([]string{
		string(commandType),
		string(exitCode),
	})).Submit(1.0)
}

// GitRequestsSearchCommits the number of requests sent to the search commit endpoint, regardless of success.
func GitRequestsSearchCommits(requestCompressed RequestCompressedType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "git_requests.search_commits", removeEmptyStrings([]string{
		string(requestCompressed),
	})).Submit(1.0)
}

// GitRequestsSearchCommitsErrors the number of requests sent to the search commit endpoint that errored, tagged by the error type.
func GitRequestsSearchCommitsErrors(errorType ErrorType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "git_requests.search_commits_errors", removeEmptyStrings(errorType)).Submit(1.0)
}

// GitRequestsObjectsPack the number of requests sent to the objects pack endpoint, tagged by the request compressed type.
func GitRequestsObjectsPack(requestCompressed RequestCompressedType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "git_requests.objects_pack", removeEmptyStrings([]string{
		string(requestCompressed),
	})).Submit(1.0)
}

// GitRequestsObjectsPackErrors the number of requests sent to the objects pack endpoint that errored, tagged by the error type.
func GitRequestsObjectsPackErrors(errorType ErrorType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "git_requests.objects_pack_errors", removeEmptyStrings(errorType)).Submit(1.0)
}

// GitRequestsSettings the number of requests sent to the settings endpoint, tagged by the request compressed type.
func GitRequestsSettings(requestCompressed RequestCompressedType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "git_requests.settings", removeEmptyStrings([]string{
		string(requestCompressed),
	})).Submit(1.0)
}

// GitRequestsSettingsErrors the number of requests sent to the settings endpoint that errored, tagged by the error type.
func GitRequestsSettingsErrors(errorType ErrorType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "git_requests.settings_errors", removeEmptyStrings(errorType)).Submit(1.0)
}

// GitRequestsSettingsResponse the number of settings responses received by CI Visibility, tagged by the settings response type.
func GitRequestsSettingsResponse(settingsResponseType SettingsResponseType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "git_requests.settings_response", removeEmptyStrings(settingsResponseType)).Submit(1.0)
}

// ITRSkippableTestsRequest the number of requests sent to the ITR skippable tests endpoint, tagged by the request compressed type.
func ITRSkippableTestsRequest(requestCompressed RequestCompressedType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "itr_skippable_tests.request", removeEmptyStrings([]string{
		string(requestCompressed),
	})).Submit(1.0)
}

// ITRSkippableTestsRequestErrors the number of requests sent to the ITR skippable tests endpoint that errored, tagged by the error type.
func ITRSkippableTestsRequestErrors(errorType ErrorType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "itr_skippable_tests.request_errors", removeEmptyStrings(errorType)).Submit(1.0)
}

// ITRSkippableTestsResponseTests the number of tests received in the ITR skippable tests response by CI Visibility.
func ITRSkippableTestsResponseTests(value float64) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "itr_skippable_tests.response_tests", nil).Submit(value)
}

// ITRSkipped the number of ITR tests skipped by CI Visibility, tagged by the event type.
func ITRSkipped(eventType TestingEventType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "itr_skipped", removeEmptyStrings(eventType)).Submit(1.0)
}

// ITRUnskippable the number of ITR tests unskippable by CI Visibility, tagged by the event type.
func ITRUnskippable(eventType TestingEventType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "itr_unskippable", removeEmptyStrings(eventType)).Submit(1.0)
}

// ITRForcedRun the number of tests or test suites that would've been skipped by ITR but were forced to run because of their unskippable status by CI Visibility.
func ITRForcedRun(eventType TestingEventType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "itr_forced_run", removeEmptyStrings(eventType)).Submit(1.0)
}

// CodeCoverageIsEmpty the number of code coverage payloads that are empty by CI Visibility.
func CodeCoverageIsEmpty() {
	telemetry.Count(telemetry.NamespaceCIVisibility, "code_coverage.is_empty", nil).Submit(1.0)
}

// CodeCoverageErrors the number of errors while processing code coverage by CI Visibility.
func CodeCoverageErrors() {
	telemetry.Count(telemetry.NamespaceCIVisibility, "code_coverage.errors", nil).Submit(1.0)
}

// KnownTestsRequest the number of requests sent to the known tests endpoint, tagged by the request compressed type.
func KnownTestsRequest(requestCompressed RequestCompressedType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "known_tests.request", removeEmptyStrings([]string{
		string(requestCompressed),
	})).Submit(1.0)
}

// KnownTestsRequestErrors the number of requests sent to the known tests endpoint that errored, tagged by the error type.
func KnownTestsRequestErrors(errorType ErrorType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "known_tests.request_errors", removeEmptyStrings(errorType)).Submit(1.0)
}

// TestManagementTestsRequest the number of requests sent to the test management tests endpoint, tagged by the request compressed type.
func TestManagementTestsRequest(requestCompressed RequestCompressedType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "test_management_tests.request", removeEmptyStrings([]string{
		string(requestCompressed),
	})).Submit(1.0)
}

// TestManagementTestsRequestErrors the number of requests sent to the test management tests endpoint that errored, tagged by the error type.
func TestManagementTestsRequestErrors(errorType ErrorType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "test_management_tests.request_errors", removeEmptyStrings(errorType)).Submit(1.0)
}

// ImpactedTestsRequest the number of requests sent to the impacted tests endpoint, tagged by the request compressed type.
func ImpactedTestsRequest(requestCompressed RequestCompressedType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "impacted_tests_detection.request", removeEmptyStrings([]string{
		string(requestCompressed),
	})).Submit(1.0)
}

// ImpactedTestsRequestErrors the number of requests sent to the impacted tests endpoint that errored, tagged by the error type.
func ImpactedTestsRequestErrors(errorType ErrorType) {
	telemetry.Count(telemetry.NamespaceCIVisibility, "impacted_tests_detection.request_errors", removeEmptyStrings(errorType)).Submit(1.0)
}

// ImpactedTestsModified the number of impacted tests that were modified by CI Visibility.
func ImpactedTestsModified() {
	telemetry.Count(telemetry.NamespaceCIVisibility, "impacted_tests_detection.is_modified", nil).Submit(1.0)
}
