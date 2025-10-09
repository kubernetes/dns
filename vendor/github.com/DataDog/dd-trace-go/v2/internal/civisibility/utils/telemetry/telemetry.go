// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package telemetry

// TestingFramework is a type for testing frameworks
type TestingFramework string

const (
	GoTestingFramework TestingFramework = "test_framework:testing"
	UnknownFramework   TestingFramework = "test_framework:unknown"
)

// TestSessionEventType is a type for test session event types
type TestSessionType []string

var (
	AppVeyorTestSessionType       TestSessionType = []string{"provider:appveyor"}
	AzurePipelinesTestSessionType TestSessionType = []string{"provider:azp"}
	BitbucketTestSessionType      TestSessionType = []string{"provider:bitbucket"}
	BitRiseTestSessionType        TestSessionType = []string{"provider:bitrise"}
	BuildKiteTestSessionType      TestSessionType = []string{"provider:buildkite"}
	CircleCiTestSessionType       TestSessionType = []string{"provider:circleci"}
	CodeFreshTestSessionType      TestSessionType = []string{"provider:codefresh"}
	GithubActionsTestSessionType  TestSessionType = []string{"provider:githubactions"}
	GitlabTestSessionType         TestSessionType = []string{"provider:gitlab"}
	JenkinsTestSessionType        TestSessionType = []string{"provider:jenkins"}
	TeamcityTestSessionType       TestSessionType = []string{"provider:teamcity"}
	TravisCiTestSessionType       TestSessionType = []string{"provider:travisci"}
	BuddyCiTestSessionType        TestSessionType = []string{"provider:buddyci"}
	AwsCodePipelineSessionType    TestSessionType = []string{"provider:aws"}
	UnsupportedTestSessionType    TestSessionType = []string{"provider:unsupported"}

	IsAutoInstrumentationTestSessionType TestSessionType = []string{"auto_injected:true"}
)

// TestingEventType is a type for testing event types
type TestingEventType []string

var (
	TestEventType    TestingEventType = []string{"event_type:test"}
	SuiteEventType   TestingEventType = []string{"event_type:suite"}
	ModuleEventType  TestingEventType = []string{"event_type:module"}
	SessionEventType TestingEventType = []string{"event_type:session"}

	UnsupportedCiEventType       TestingEventType = []string{"is_unsupported_ci"}
	HasCodeOwnerEventType        TestingEventType = []string{"has_codeowner"}
	IsNewEventType               TestingEventType = []string{"is_new:true"}
	IsRetryEventType             TestingEventType = []string{"is_retry:true"}
	EfdAbortSlowEventType        TestingEventType = []string{"early_flake_detection_abort_reason:slow"}
	IsBenchmarkEventType         TestingEventType = []string{"is_benchmark"}
	IsAttemptToFixEventType      TestingEventType = []string{"is_attempt_to_fix:true"}
	IsQuarantinedEventType       TestingEventType = []string{"is_quarantined:true"}
	IsDisabledEventType          TestingEventType = []string{"is_disabled:true"}
	HasFailedAllRetriesEventType TestingEventType = []string{"has_failed_all_retries:true"}
)

// CoverageLibraryType is a type for coverage library types
type CoverageLibraryType string

const (
	DefaultCoverageLibraryType CoverageLibraryType = "library:default"
	UnknownCoverageLibraryType CoverageLibraryType = "library:unknown"
)

// EndpointType is a type for endpoint types
type EndpointType string

const (
	TestCycleEndpointType    EndpointType = "endpoint:test_cycle"
	CodeCoverageEndpointType EndpointType = "endpoint:code_coverage"
)

// ErrorType is a type for error types
type ErrorType []string

var (
	TimeoutErrorType       ErrorType = []string{"error_type:timeout"}
	NetworkErrorType       ErrorType = []string{"error_type:network"}
	StatusCodeErrorType    ErrorType = []string{"error_type:status_code"}
	StatusCode4xxErrorType ErrorType = []string{"error_type:status_code_4xx_response"}
	StatusCode5xxErrorType ErrorType = []string{"error_type:status_code_5xx_response"}
	StatusCode400ErrorType ErrorType = []string{"error_type:status_code_4xx_response", "status_code:400"}
	StatusCode401ErrorType ErrorType = []string{"error_type:status_code_4xx_response", "status_code:401"}
	StatusCode403ErrorType ErrorType = []string{"error_type:status_code_4xx_response", "status_code:403"}
	StatusCode404ErrorType ErrorType = []string{"error_type:status_code_4xx_response", "status_code:404"}
	StatusCode408ErrorType ErrorType = []string{"error_type:status_code_4xx_response", "status_code:408"}
	StatusCode429ErrorType ErrorType = []string{"error_type:status_code_4xx_response", "status_code:429"}
)

// CommandType is a type for commands types
type CommandType string

const (
	NotSpecifiedCommandsType              CommandType = ""
	GetRepositoryCommandsType             CommandType = "command:get_repository"
	GetBranchCommandsType                 CommandType = "command:get_branch"
	GetRemoteCommandsType                 CommandType = "command:get_remote"
	GetRemoteUpstreamTrackingCommandsType CommandType = "command:get_remote_upstream_tracking"
	GetHeadCommandsType                   CommandType = "command:get_head"
	CheckShallowCommandsType              CommandType = "command:check_shallow"
	UnshallowCommandsType                 CommandType = "command:unshallow"
	GetLocalCommitsCommandsType           CommandType = "command:get_local_commits"
	GetObjectsCommandsType                CommandType = "command:get_objects"
	PackObjectsCommandsType               CommandType = "command:pack_objects"
	DiffCommandType                       CommandType = "command:diff"
	ShowRefCommandType                    CommandType = "command:show_ref"
	LsRemoteHeadsCommandType              CommandType = "command:ls_remote_heads"
	FetchCommandType                      CommandType = "command:fetch"
	ForEachRefCommandType                 CommandType = "command:for_each_ref"
	MergeBaseCommandType                  CommandType = "command:merge_base"
	RevListCommandType                    CommandType = "command:rev_list"
	SymbolicRefCommandType                CommandType = "command:symbolic_ref"
	GetWorkingDirectoryCommandType        CommandType = "command:get_working_directory"
	GetGitCommitInfoCommandType           CommandType = "command:get_git_info"
	GitAddPermissionCommandType           CommandType = "command:git_add_permission"
)

// CommandExitCodeType is a type for command exit codes
type CommandExitCodeType string

const (
	MissingCommandExitCode  CommandExitCodeType = "exit_code:missing"
	UnknownCommandExitCode  CommandExitCodeType = "exit_code:unknown"
	ECMinus1CommandExitCode CommandExitCodeType = "exit_code:-1"
	EC1CommandExitCode      CommandExitCodeType = "exit_code:1"
	EC2CommandExitCode      CommandExitCodeType = "exit_code:2"
	EC127CommandExitCode    CommandExitCodeType = "exit_code:127"
	EC128CommandExitCode    CommandExitCodeType = "exit_code:128"
	EC129CommandExitCode    CommandExitCodeType = "exit_code:129"
)

// RequestCompressedType is a type for request compressed types
type RequestCompressedType string

const (
	UncompressedRequestCompressedType RequestCompressedType = ""
	CompressedRequestCompressedType   RequestCompressedType = "rq_compressed:true"
)

// ResponseCompressedType is a type for response compressed types
type ResponseCompressedType string

const (
	UncompressedResponseCompressedType ResponseCompressedType = ""
	CompressedResponseCompressedType   ResponseCompressedType = "rs_compressed:true"
)

// SettingsResponseType is a type for settings response types
type SettingsResponseType []string

var (
	CoverageEnabledSettingsResponseType         SettingsResponseType = []string{"coverage_enabled"}
	ItrSkipEnabledSettingsResponseType          SettingsResponseType = []string{"itrskip_enabled"}
	EfdEnabledSettingsResponseType              SettingsResponseType = []string{"early_flake_detection_enabled:true"}
	FlakyTestRetriesEnabledSettingsResponseType SettingsResponseType = []string{"flaky_test_retries_enabled:true"}
	TestManagementEnabledSettingsResponseType   SettingsResponseType = []string{"test_management_enabled:true"}
)

// removeEmptyStrings removes empty string values from a slice.
func removeEmptyStrings(s []string) []string {
	result := make([]string, len(s))
	n := 0
	for _, str := range s {
		if str != "" {
			result[n] = str
			n++
		}
	}
	return result[:n]
}
