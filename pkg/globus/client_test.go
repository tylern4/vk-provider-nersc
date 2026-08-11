package globus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type staticTokenSource string

func (s staticTokenSource) Token(context.Context) (string, error) { return string(s), nil }

func TestClientStartsAndChecksTransfer(t *testing.T) {
	var transferBody struct {
		DataType            string `json:"DATA_TYPE"`
		SubmissionID        string `json:"submission_id"`
		SourceEndpoint      string `json:"source_endpoint"`
		DestinationEndpoint string `json:"destination_endpoint"`
		Data                []struct {
			DataType        string `json:"DATA_TYPE"`
			SourcePath      string `json:"source_path"`
			DestinationPath string `json:"destination_path"`
			Recursive       bool   `json:"recursive"`
		} `json:"DATA"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer transfer-token" {
			t.Errorf("authorization = %q", got)
		}
		switch r.URL.Path {
		case "/v0.10/submission_id":
			_, _ = w.Write([]byte(`{"value":"submission-1"}`))
		case "/v0.10/transfer":
			if err := json.NewDecoder(r.Body).Decode(&transferBody); err != nil {
				t.Errorf("decode transfer: %v", err)
			}
			_, _ = w.Write([]byte(`{"task_id":"task-1"}`))
		case "/v0.10/task/task-1":
			_, _ = w.Write([]byte(`{"task_id":"task-1","status":"SUCCEEDED"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClientWithOptions(server.URL+"/v0.10/", staticTokenSource("transfer-token"), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	taskID, err := client.StartTransfer(context.Background(), TransferRequest{
		SourceCollection: "source-id", DestinationCollection: "destination-id",
		SourcePath: "/input", DestinationPath: "/output", Recursive: true,
	})
	if err != nil {
		t.Fatalf("StartTransfer: %v", err)
	}
	if taskID != "task-1" {
		t.Fatalf("task id = %q", taskID)
	}
	if transferBody.DataType != "transfer" || transferBody.SubmissionID != "submission-1" {
		t.Fatalf("transfer body = %+v", transferBody)
	}
	if transferBody.SourceEndpoint != "source-id" || transferBody.DestinationEndpoint != "destination-id" {
		t.Fatalf("collections = %s -> %s", transferBody.SourceEndpoint, transferBody.DestinationEndpoint)
	}
	if len(transferBody.Data) != 1 || !transferBody.Data[0].Recursive {
		t.Fatalf("transfer items = %+v", transferBody.Data)
	}

	task, err := client.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if done, failed := task.IsComplete(); !done || failed {
		t.Fatalf("task completion = %t/%t", done, failed)
	}
}

func TestClientIncludesGlobusAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":"PermissionDenied","message":"collection access denied"}`))
	}))
	defer server.Close()
	client, err := NewClientWithOptions(server.URL+"/v0.10/", staticTokenSource("token"), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetTask(context.Background(), "task-1")
	if err == nil || !strings.Contains(err.Error(), "collection access denied") {
		t.Fatalf("error = %v", err)
	}
}
