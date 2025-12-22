// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

const (
	resourceTypeDatasets    = "datasets"
	resourceTypeExperiments = "experiments"
	resourceTypeProjects    = "projects"
)

// ---------- Resources ----------

type DatasetView struct {
	ID             string
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Metadata       map[string]any `json:"metadata"`
	CurrentVersion int            `json:"current_version"`
}

type DatasetCreate struct {
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type DatasetRecordView struct {
	ID             string
	Input          any `json:"input"`
	ExpectedOutput any `json:"expected_output"`
	Metadata       any `json:"metadata"`
	Version        int `json:"version"`
}

type ProjectView struct {
	ID   string
	Name string `json:"name"`
}

type ExperimentView struct {
	ID             string
	ProjectID      string         `json:"project_id"`
	DatasetID      string         `json:"dataset_id"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Metadata       map[string]any `json:"metadata"`
	Config         map[string]any `json:"config"`
	DatasetVersion int            `json:"dataset_version"`
	EnsureUnique   bool           `json:"ensure_unique"`
}

type DatasetRecordCreate struct {
	Input          any `json:"input,omitempty"`
	ExpectedOutput any `json:"expected_output,omitempty"`
	Metadata       any `json:"metadata,omitempty"`
}

type DatasetRecordUpdate struct {
	ID             string `json:"id"`
	Input          any    `json:"input,omitempty"`
	ExpectedOutput *any   `json:"expected_output,omitempty"`
	Metadata       any    `json:"metadata,omitempty"`
}

type ErrorMessage struct {
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
	Stack   string `json:"stack,omitempty"`
}

// ---------- Requests ----------

type Request[T any] struct {
	Data RequestData[T] `json:"data"`
}

type RequestData[T any] struct {
	Type       string `json:"type"`
	Attributes T      `json:"attributes"`
}

type RequestAttributesDatasetCreateRecords struct {
	Records []DatasetRecordCreate `json:"records,omitempty"`
}

type RequestAttributesDatasetDelete struct {
	DatasetIDs []string `json:"dataset_ids,omitempty"`
}

type RequestAttributesDatasetBatchUpdate struct {
	InsertRecords []DatasetRecordCreate `json:"insert_records,omitempty"`
	UpdateRecords []DatasetRecordUpdate `json:"update_records,omitempty"`
	DeleteRecords []string              `json:"delete_records,omitempty"`
	Deduplicate   *bool                 `json:"deduplicate,omitempty"`
}

type RequestAttributesProjectCreate struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type RequestAttributesExperimentCreate struct {
	ProjectID      string         `json:"project_id,omitempty"`
	DatasetID      string         `json:"dataset_id,omitempty"`
	Name           string         `json:"name,omitempty"`
	Description    string         `json:"description,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	Config         map[string]any `json:"config,omitempty"`
	DatasetVersion int            `json:"dataset_version,omitempty"`
	EnsureUnique   bool           `json:"ensure_unique,omitempty"`
}

type RequestAttributesExperimentPushEvents struct {
	Scope   string                      `json:"scope,omitempty"`
	Metrics []ExperimentEvalMetricEvent `json:"metrics,omitempty"`
	Tags    []string                    `json:"tags,omitempty"`
}

type ExperimentEvalMetricEvent struct {
	MetricSource     string        `json:"metric_source,omitempty"`
	SpanID           string        `json:"span_id,omitempty"`
	TraceID          string        `json:"trace_id,omitempty"`
	TimestampMS      int64         `json:"timestamp_ms,omitempty"`
	MetricType       string        `json:"metric_type,omitempty"`
	Label            string        `json:"label,omitempty"`
	CategoricalValue *string       `json:"categorical_value,omitempty"`
	ScoreValue       *float64      `json:"score_value,omitempty"`
	BooleanValue     *bool         `json:"boolean_value,omitempty"`
	Error            *ErrorMessage `json:"error,omitempty"`
	Tags             []string      `json:"tags,omitempty"`
	ExperimentID     string        `json:"experiment_id,omitempty"`
}

type (
	CreateDatasetRequest        = Request[DatasetCreate]
	DeleteDatasetRequest        = Request[RequestAttributesDatasetDelete]
	CreateDatasetRecordsRequest = Request[RequestAttributesDatasetCreateRecords]
	BatchUpdateDatasetRequest   = Request[RequestAttributesDatasetBatchUpdate]

	CreateProjectRequest = Request[RequestAttributesProjectCreate]

	CreateExperimentRequest     = Request[RequestAttributesExperimentCreate]
	PushExperimentEventsRequest = Request[RequestAttributesExperimentPushEvents]
)

// ---------- Responses ----------

type Response[T any] struct {
	Data ResponseData[T] `json:"data"`
}

type ResponseMeta struct {
	After string `json:"after,omitempty"` // Cursor for next page
}

type ResponseList[T any] struct {
	Data []ResponseData[T] `json:"data"`
	Meta ResponseMeta      `json:"meta,omitempty"`
}

type ResponseData[T any] struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Attributes T      `json:"attributes"`
}

type (
	GetDatasetResponse    = ResponseList[DatasetView]
	CreateDatasetResponse = Response[DatasetView]
	UpdateDatasetResponse = Response[DatasetView]

	GetDatasetRecordsResponse    = ResponseList[DatasetRecordView]
	CreateDatasetRecordsResponse = ResponseList[DatasetRecordView]
	UpdateDatasetRecordsResponse = ResponseList[DatasetRecordView]
	BatchUpdateDatasetResponse   = ResponseList[DatasetRecordView]

	CreateProjectResponse = Response[ProjectView]

	CreateExperimentResponse = Response[ExperimentView]
)

func (c *Transport) GetDatasetByName(ctx context.Context, name, projectID string) (*DatasetView, error) {
	q := url.Values{}
	q.Set("filter[name]", name)
	datasetPath := fmt.Sprintf("%s/%s/datasets?%s", endpointPrefixDNE, url.PathEscape(projectID), q.Encode())
	method := http.MethodGet

	result, err := c.jsonRequest(ctx, method, datasetPath, subdomainDNE, nil, defaultTimeout)
	if err != nil {
		return nil, err
	}
	if result.statusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", result.statusCode, string(result.body))
	}

	var datasetResp GetDatasetResponse
	if err := json.Unmarshal(result.body, &datasetResp); err != nil {
		return nil, fmt.Errorf("failed to decode json response: %w", err)
	}
	if len(datasetResp.Data) == 0 {
		return nil, ErrDatasetNotFound
	}
	ds := datasetResp.Data[0].Attributes
	ds.ID = datasetResp.Data[0].ID
	return &ds, nil
}

func (c *Transport) CreateDataset(ctx context.Context, name, description, projectID string) (*DatasetView, error) {
	_, err := c.GetDatasetByName(ctx, name, projectID)
	if err == nil {
		return nil, errors.New("dataset already exists")
	}
	if !errors.Is(err, ErrDatasetNotFound) {
		return nil, err
	}

	path := fmt.Sprintf("%s/%s/datasets", endpointPrefixDNE, url.PathEscape(projectID))
	method := http.MethodPost
	body := CreateDatasetRequest{
		Data: RequestData[DatasetCreate]{
			Type: resourceTypeDatasets,
			Attributes: DatasetCreate{
				Name:        name,
				Description: description,
			},
		},
	}
	result, err := c.jsonRequest(ctx, method, path, subdomainDNE, body, defaultTimeout)
	if err != nil {
		return nil, err
	}
	log.Debug("llmobs: create dataset success (status code: %d)", result.statusCode)

	var resp CreateDatasetResponse
	if err := json.Unmarshal(result.body, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode json response: %w", err)
	}
	id := resp.Data.ID
	dataset := resp.Data.Attributes
	dataset.ID = id
	return &dataset, nil
}

func (c *Transport) DeleteDataset(ctx context.Context, datasetIDs ...string) error {
	path := endpointPrefixDNE + "/datasets/delete"
	method := http.MethodPost
	body := DeleteDatasetRequest{
		Data: RequestData[RequestAttributesDatasetDelete]{
			Type: resourceTypeDatasets,
			Attributes: RequestAttributesDatasetDelete{
				DatasetIDs: datasetIDs,
			},
		},
	}

	result, err := c.jsonRequest(ctx, method, path, subdomainDNE, body, defaultTimeout)
	if err != nil {
		return err
	}
	if result.statusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d: %s", result.statusCode, string(result.body))
	}
	return nil
}

func (c *Transport) BatchUpdateDataset(
	ctx context.Context,
	datasetID string,
	insert []DatasetRecordCreate,
	update []DatasetRecordUpdate,
	delete []string,
) (int, []string, error) {
	path := fmt.Sprintf("%s/datasets/%s/batch_update", endpointPrefixDNE, url.PathEscape(datasetID))
	method := http.MethodPost
	body := BatchUpdateDatasetRequest{
		Data: RequestData[RequestAttributesDatasetBatchUpdate]{
			Type: resourceTypeDatasets,
			Attributes: RequestAttributesDatasetBatchUpdate{
				InsertRecords: insert,
				UpdateRecords: update,
				DeleteRecords: delete,
				Deduplicate:   AnyPtr(false),
			},
		},
	}

	result, err := c.jsonRequest(ctx, method, path, subdomainDNE, body, defaultTimeout)
	if err != nil {
		return -1, nil, err
	}
	if result.statusCode != http.StatusOK {
		return -1, nil, fmt.Errorf("unexpected status %d: %s", result.statusCode, string(result.body))
	}

	var resp BatchUpdateDatasetResponse
	if err := json.Unmarshal(result.body, &resp); err != nil {
		return -1, nil, fmt.Errorf("failed to decode json response: %w", err)
	}

	// FIXME: we don't get version numbers in responses to deletion requests
	// TODO(rarguelloF): the backend could return a better response here...
	var (
		newDatasetVersion = -1
		newRecordIDs      []string
	)
	if len(resp.Data) > 0 {
		if resp.Data[0].Attributes.Version > 0 {
			newDatasetVersion = resp.Data[0].Attributes.Version
		}
	}
	if len(resp.Data) == len(insert)+len(update) {
		// new records are at the end of the slice
		for _, rec := range resp.Data[len(update):] {
			newRecordIDs = append(newRecordIDs, rec.ID)
		}
	} else {
		log.Warn("llmobs/internal/transport: BatchUpdateDataset: expected %d records in response, got %d", len(insert)+len(update), len(resp.Data))
	}
	return newDatasetVersion, newRecordIDs, nil
}

// GetDatasetRecordsPage fetches a single page of records for the given dataset.
// Returns the records, the cursor for the next page (empty string if no more pages), and any error.
func (c *Transport) GetDatasetRecordsPage(ctx context.Context, datasetID, cursor string) ([]DatasetRecordView, string, error) {
	method := http.MethodGet
	recordsPath := fmt.Sprintf("%s/datasets/%s/records", endpointPrefixDNE, url.PathEscape(datasetID))

	if cursor != "" {
		recordsPath = fmt.Sprintf("%s?page[cursor]=%s", recordsPath, url.QueryEscape(cursor))
	}

	result, err := c.jsonRequest(ctx, method, recordsPath, subdomainDNE, nil, getDatasetRecordsTimeout)
	if err != nil {
		return nil, "", err
	}
	if result.statusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status %d: %s", result.statusCode, string(result.body))
	}

	var recordsResp GetDatasetRecordsResponse
	if err := json.Unmarshal(result.body, &recordsResp); err != nil {
		return nil, "", fmt.Errorf("failed to decode json response: %w", err)
	}

	records := make([]DatasetRecordView, 0, len(recordsResp.Data))
	for _, r := range recordsResp.Data {
		rec := r.Attributes
		rec.ID = r.ID
		records = append(records, rec)
	}

	return records, recordsResp.Meta.After, nil
}

// GetDatasetWithRecords fetches the given Dataset and all its records from DataDog.
// This eagerly fetches all pages of records.
func (c *Transport) GetDatasetWithRecords(ctx context.Context, name, projectID string) (*DatasetView, []DatasetRecordView, error) {
	// 1) Fetch dataset by name
	ds, err := c.GetDatasetByName(ctx, name, projectID)
	if err != nil {
		return nil, nil, err
	}

	// 2) Fetch all records with pagination support
	var allRecords []DatasetRecordView
	nextCursor := ""
	pageNum := 0

	for {
		log.Debug("llmobs/transport: fetching dataset records page %d", pageNum)

		records, cursor, err := c.GetDatasetRecordsPage(ctx, ds.ID, nextCursor)
		if err != nil {
			return nil, nil, fmt.Errorf("get dataset records failed on page %d: %w", pageNum, err)
		}

		allRecords = append(allRecords, records...)

		nextCursor = cursor
		if nextCursor == "" {
			break
		}
		pageNum++
	}

	log.Debug("llmobs/transport: fetched %d records across %d pages for dataset %q", len(allRecords), pageNum+1, name)
	return ds, allRecords, nil
}

func (c *Transport) GetOrCreateProject(ctx context.Context, name string) (*ProjectView, error) {
	path := endpointPrefixDNE + "/projects"
	method := http.MethodPost

	body := CreateProjectRequest{
		Data: RequestData[RequestAttributesProjectCreate]{
			Type: resourceTypeProjects,
			Attributes: RequestAttributesProjectCreate{
				Name:        name,
				Description: "",
			},
		},
	}
	result, err := c.jsonRequest(ctx, method, path, subdomainDNE, body, defaultTimeout)
	if err != nil {
		return nil, err
	}
	if result.statusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", result.statusCode, string(result.body))
	}

	var resp CreateProjectResponse
	if err := json.Unmarshal(result.body, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode json response: %w", err)
	}

	project := resp.Data.Attributes
	project.ID = resp.Data.ID
	return &project, nil
}

func (c *Transport) CreateExperiment(
	ctx context.Context,
	name, datasetID, projectID string,
	datasetVersion int,
	expConfig map[string]any,
	tags []string,
	description string,
) (*ExperimentView, error) {
	path := endpointPrefixDNE + "/experiments"
	method := http.MethodPost

	if expConfig == nil {
		expConfig = map[string]interface{}{}
	}
	meta := map[string]interface{}{"tags": tags}
	body := CreateExperimentRequest{
		Data: RequestData[RequestAttributesExperimentCreate]{
			Type: resourceTypeExperiments,
			Attributes: RequestAttributesExperimentCreate{
				ProjectID:      projectID,
				DatasetID:      datasetID,
				Name:           name,
				Description:    description,
				Metadata:       meta,
				Config:         expConfig,
				DatasetVersion: datasetVersion,
				EnsureUnique:   true,
			},
		},
	}

	result, err := c.jsonRequest(ctx, method, path, subdomainDNE, body, defaultTimeout)
	if err != nil {
		return nil, err
	}
	if result.statusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", result.statusCode, string(result.body))
	}

	var resp CreateExperimentResponse
	if err := json.Unmarshal(result.body, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode json response: %w", err)
	}
	exp := resp.Data.Attributes
	exp.ID = resp.Data.ID

	return &exp, nil
}

func (c *Transport) PushExperimentEvents(
	ctx context.Context,
	experimentID string,
	metrics []ExperimentEvalMetricEvent,
	tags []string,
) error {
	path := fmt.Sprintf("%s/experiments/%s/events", endpointPrefixDNE, url.PathEscape(experimentID))
	method := http.MethodPost

	body := PushExperimentEventsRequest{
		Data: RequestData[RequestAttributesExperimentPushEvents]{
			Type: resourceTypeExperiments,
			Attributes: RequestAttributesExperimentPushEvents{
				Scope:   resourceTypeExperiments,
				Metrics: metrics,
				Tags:    tags,
			},
		},
	}

	result, err := c.jsonRequest(ctx, method, path, subdomainDNE, body, defaultTimeout)
	if err != nil {
		return err
	}
	if result.statusCode != http.StatusOK && result.statusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status %d: %s", result.statusCode, string(result.body))
	}
	return nil
}

// BulkUploadDataset uploads dataset records via CSV file upload.
// This is more efficient for large datasets (>5MB of changes).
func (c *Transport) BulkUploadDataset(ctx context.Context, datasetID string, records []DatasetRecordView) error {
	// Create CSV in memory
	var csvBuf bytes.Buffer
	csvWriter := csv.NewWriter(&csvBuf)

	// Write header
	if err := csvWriter.Write([]string{"input", "expected_output", "metadata"}); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write records
	for _, rec := range records {
		inputJSON, err := json.Marshal(rec.Input)
		if err != nil {
			return fmt.Errorf("failed to marshal input: %w", err)
		}
		outputJSON, err := json.Marshal(rec.ExpectedOutput)
		if err != nil {
			return fmt.Errorf("failed to marshal expected_output: %w", err)
		}
		metadataJSON, err := json.Marshal(rec.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}

		if err := csvWriter.Write([]string{
			string(inputJSON),
			string(outputJSON),
			string(metadataJSON),
		}); err != nil {
			return fmt.Errorf("failed to write CSV record: %w", err)
		}
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return fmt.Errorf("CSV writer error: %w", err)
	}

	// Create multipart body
	boundary := "----------boundary------"
	crlf := "\r\n"
	filename := "dataset_upload.csv"

	var body bytes.Buffer
	body.WriteString("--" + boundary + crlf)
	body.WriteString(fmt.Sprintf(`Content-Disposition: form-data; name="file"; filename="%s"`, filename) + crlf)
	body.WriteString("Content-Type: text/csv" + crlf)
	body.WriteString(crlf)
	body.Write(csvBuf.Bytes())
	body.WriteString(crlf)
	body.WriteString("--" + boundary + "--" + crlf)

	path := fmt.Sprintf("%s/datasets/%s/records/upload", endpointPrefixDNE, url.PathEscape(datasetID))
	contentType := fmt.Sprintf("multipart/form-data; boundary=%s", boundary)

	result, err := c.request(ctx, http.MethodPost, path, subdomainDNE, bytes.NewReader(body.Bytes()), contentType, bulkUploadTimeout)
	if err != nil {
		return err
	}
	if result.statusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d: %s", result.statusCode, string(result.body))
	}

	log.Debug("llmobs/transport: successfully bulk uploaded %d records to dataset %q: %s", len(records), datasetID, string(result.body))
	return nil
}
