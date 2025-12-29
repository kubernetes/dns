// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	_ "unsafe" // for go:linkname

	"github.com/DataDog/dd-trace-go/v2/internal/civisibility/constants"
	"github.com/DataDog/dd-trace-go/v2/internal/env"
	logger "github.com/DataDog/dd-trace-go/v2/internal/log"
)

type (
	/*
		{
		  "ci.workspace_path": "ci.workspace_path",
		  "git.repository_url": "git.repository_url",
		  "git.commit.sha": "git.commit.sha",
		  "git.branch": "user-supplied-branch",
		  "git.tag": "user-supplied-tag",
		  "git.commit.author.date": "usersupplied-authordate",
		  "git.commit.author.name": "usersupplied-authorname",
		  "git.commit.author.email": "usersupplied-authoremail",
		  "git.commit.committer.date": "usersupplied-comitterdate",
		  "git.commit.committer.name": "usersupplied-comittername",
		  "git.commit.committer.email": "usersupplied-comitteremail",
		  "git.commit.message": "usersupplied-message",
		  "ci.provider.name": "",
		  "ci.pipeline.id": "",
		  "ci.pipeline.url": "",
		  "ci.pipeline.name": "",
		  "ci.pipeline.number": "",
		  "ci.stage.name": "",
		  "ci.job.name": "",
		  "ci.job.url": "",
		  "ci.node.name": "",
		  "ci.node.labels": "",
		  "_dd.ci.env_vars": ""
		}
	*/

	// fileEnvironmentalData represents the environmental data for the complete test session.
	fileEnvironmentalData struct {
		WorkspacePath        string `json:"ci.workspace_path,omitempty"`
		RepositoryURL        string `json:"git.repository_url,omitempty"`
		CommitSHA            string `json:"git.commit.sha,omitempty"`
		Branch               string `json:"git.branch,omitempty"`
		Tag                  string `json:"git.tag,omitempty"`
		CommitAuthorDate     string `json:"git.commit.author.date,omitempty"`
		CommitAuthorName     string `json:"git.commit.author.name,omitempty"`
		CommitAuthorEmail    string `json:"git.commit.author.email,omitempty"`
		CommitCommitterDate  string `json:"git.commit.committer.date,omitempty"`
		CommitCommitterName  string `json:"git.commit.committer.name,omitempty"`
		CommitCommitterEmail string `json:"git.commit.committer.email,omitempty"`
		CommitMessage        string `json:"git.commit.message,omitempty"`
		CIProviderName       string `json:"ci.provider.name,omitempty"`
		CIPipelineID         string `json:"ci.pipeline.id,omitempty"`
		CIPipelineURL        string `json:"ci.pipeline.url,omitempty"`
		CIPipelineName       string `json:"ci.pipeline.name,omitempty"`
		CIPipelineNumber     string `json:"ci.pipeline.number,omitempty"`
		CIStageName          string `json:"ci.stage.name,omitempty"`
		CIJobName            string `json:"ci.job.name,omitempty"`
		CIJobURL             string `json:"ci.job.url,omitempty"`
		CINodeName           string `json:"ci.node.name,omitempty"`
		CINodeLabels         string `json:"ci.node.labels,omitempty"`
		DDCIEnvVars          string `json:"_dd.ci.env_vars,omitempty"`
	}
)

// getEnvironmentalData reads the environmental data from the file.
//
//go:linkname getEnvironmentalData
func getEnvironmentalData() *fileEnvironmentalData {
	envDataFileName := getEnvDataFileName()
	if _, err := os.Stat(envDataFileName); os.IsNotExist(err) {
		logger.Debug("civisibility: reading environmental data from %s not found.", envDataFileName)
		return nil
	}
	file, err := os.Open(envDataFileName)
	if err != nil {
		logger.Error("civisibility: error reading environmental data from %s: %v", envDataFileName, err.Error())
		return nil
	}
	defer file.Close()
	var envData fileEnvironmentalData
	if err := json.NewDecoder(file).Decode(&envData); err != nil {
		logger.Error("civisibility: error decoding environmental data from %s: %v", envDataFileName, err.Error())
		return nil
	}
	logger.Debug("civisibility: loaded environmental data from %s", envDataFileName)
	return &envData
}

// getEnvDataFileName returns the environmental data file name.
//
//go:linkname getEnvDataFileName
func getEnvDataFileName() string {
	envDataFileName := strings.TrimSpace(env.Get(constants.CIVisibilityEnvironmentDataFilePath))
	if envDataFileName != "" {
		return envDataFileName
	}
	cmd := filepath.Base(os.Args[0])
	cmdWithoutExt := strings.TrimSuffix(cmd, filepath.Ext(cmd))
	folder := filepath.Dir(os.Args[0])
	return filepath.Join(folder, cmdWithoutExt+".env.json")
}

// applyEnvironmentalDataIfRequired applies the environmental data to the given tags if required.
//
//go:linkname applyEnvironmentalDataIfRequired
func applyEnvironmentalDataIfRequired(tags map[string]string) {
	if tags == nil {
		return
	}
	envData := getEnvironmentalData()
	if envData == nil {
		logger.Debug("civisibility: no environmental data found")
		return
	}

	logger.Debug("civisibility: applying environmental data")

	if envData.WorkspacePath != "" && tags[constants.CIWorkspacePath] == "" {
		tags[constants.CIWorkspacePath] = envData.WorkspacePath
	}

	if envData.RepositoryURL != "" && tags[constants.GitRepositoryURL] == "" {
		tags[constants.GitRepositoryURL] = envData.RepositoryURL
	}

	if envData.CommitSHA != "" && tags[constants.GitCommitSHA] == "" {
		tags[constants.GitCommitSHA] = envData.CommitSHA
	}

	if envData.Branch != "" && tags[constants.GitBranch] == "" {
		tags[constants.GitBranch] = envData.Branch
	}

	if envData.Tag != "" && tags[constants.GitTag] == "" {
		tags[constants.GitTag] = envData.Tag
	}

	if envData.CommitAuthorDate != "" && tags[constants.GitCommitAuthorDate] == "" {
		tags[constants.GitCommitAuthorDate] = envData.CommitAuthorDate
	}

	if envData.CommitAuthorName != "" && tags[constants.GitCommitAuthorName] == "" {
		tags[constants.GitCommitAuthorName] = envData.CommitAuthorName
	}

	if envData.CommitAuthorEmail != "" && tags[constants.GitCommitAuthorEmail] == "" {
		tags[constants.GitCommitAuthorEmail] = envData.CommitAuthorEmail
	}

	if envData.CommitCommitterDate != "" && tags[constants.GitCommitCommitterDate] == "" {
		tags[constants.GitCommitCommitterDate] = envData.CommitCommitterDate
	}

	if envData.CommitCommitterName != "" && tags[constants.GitCommitCommitterName] == "" {
		tags[constants.GitCommitCommitterName] = envData.CommitCommitterName
	}

	if envData.CommitCommitterEmail != "" && tags[constants.GitCommitCommitterEmail] == "" {
		tags[constants.GitCommitCommitterEmail] = envData.CommitCommitterEmail
	}

	if envData.CommitMessage != "" && tags[constants.GitCommitMessage] == "" {
		tags[constants.GitCommitMessage] = envData.CommitMessage
	}

	if envData.CIProviderName != "" && tags[constants.CIProviderName] == "" {
		tags[constants.CIProviderName] = envData.CIProviderName
	}

	if envData.CIPipelineID != "" && tags[constants.CIPipelineID] == "" {
		tags[constants.CIPipelineID] = envData.CIPipelineID
	}

	if envData.CIPipelineURL != "" && tags[constants.CIPipelineURL] == "" {
		tags[constants.CIPipelineURL] = envData.CIPipelineURL
	}

	if envData.CIPipelineName != "" && tags[constants.CIPipelineName] == "" {
		tags[constants.CIPipelineName] = envData.CIPipelineName
	}

	if envData.CIPipelineNumber != "" && tags[constants.CIPipelineNumber] == "" {
		tags[constants.CIPipelineNumber] = envData.CIPipelineNumber
	}

	if envData.CIStageName != "" && tags[constants.CIStageName] == "" {
		tags[constants.CIStageName] = envData.CIStageName
	}

	if envData.CIJobName != "" && tags[constants.CIJobName] == "" {
		tags[constants.CIJobName] = envData.CIJobName
	}

	if envData.CIJobURL != "" && tags[constants.CIJobURL] == "" {
		tags[constants.CIJobURL] = envData.CIJobURL
	}

	if envData.CINodeName != "" && tags[constants.CINodeName] == "" {
		tags[constants.CINodeName] = envData.CINodeName
	}

	if envData.CINodeLabels != "" && tags[constants.CINodeLabels] == "" {
		tags[constants.CINodeLabels] = envData.CINodeLabels
	}

	if envData.DDCIEnvVars != "" && tags[constants.CIEnvVars] == "" {
		tags[constants.CIEnvVars] = envData.DDCIEnvVars
	}
}

// createEnvironmentalDataFromTags creates a fileEnvironmentalData object from the given tags.
//
//go:linkname createEnvironmentalDataFromTags
func createEnvironmentalDataFromTags(tags map[string]string) *fileEnvironmentalData {
	if tags == nil {
		return nil
	}

	return &fileEnvironmentalData{
		WorkspacePath:        tags[constants.CIWorkspacePath],
		RepositoryURL:        tags[constants.GitRepositoryURL],
		CommitSHA:            tags[constants.GitCommitSHA],
		Branch:               tags[constants.GitBranch],
		Tag:                  tags[constants.GitTag],
		CommitAuthorDate:     tags[constants.GitCommitAuthorDate],
		CommitAuthorName:     tags[constants.GitCommitAuthorName],
		CommitAuthorEmail:    tags[constants.GitCommitAuthorEmail],
		CommitCommitterDate:  tags[constants.GitCommitCommitterDate],
		CommitCommitterName:  tags[constants.GitCommitCommitterName],
		CommitCommitterEmail: tags[constants.GitCommitCommitterEmail],
		CommitMessage:        tags[constants.GitCommitMessage],
		CIProviderName:       tags[constants.CIProviderName],
		CIPipelineID:         tags[constants.CIPipelineID],
		CIPipelineURL:        tags[constants.CIPipelineURL],
		CIPipelineName:       tags[constants.CIPipelineName],
		CIPipelineNumber:     tags[constants.CIPipelineNumber],
		CIStageName:          tags[constants.CIStageName],
		CIJobName:            tags[constants.CIJobName],
		CIJobURL:             tags[constants.CIJobURL],
		CINodeName:           tags[constants.CINodeName],
		CINodeLabels:         tags[constants.CINodeLabels],
		DDCIEnvVars:          tags[constants.CIEnvVars],
	}
}

// writeEnvironmentalDataToFile writes the environmental data to a file.
//
//go:linkname writeEnvironmentalDataToFile
func writeEnvironmentalDataToFile(filePath string, tags map[string]string) error {
	envData := createEnvironmentalDataFromTags(tags)
	if envData == nil {
		return nil
	}

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(envData)
}
