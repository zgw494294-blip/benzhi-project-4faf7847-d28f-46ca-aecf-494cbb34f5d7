package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"heritage-tree-relocation-permit/internal/application"
	"heritage-tree-relocation-permit/internal/storage/journal"
	"heritage-tree-relocation-permit/internal/webassets"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	repository, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	return NewHandler(application.NewService(repository, application.RealClock{}, nil), webassets.NewHandler())
}

func TestCreateRejectsUnknownJSONField(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodPost, "/api/cases", bytes.NewBufferString(`{"idempotencyKey":"create-http-0001","unexpected":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，实际 %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Correlation-ID") == "" {
		t.Fatal("响应缺少关联标识")
	}
}

func TestFrontendAndCaseQueries(t *testing.T) {
	handler := testHandler(t)
	pageRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	pageResponse := httptest.NewRecorder()
	handler.ServeHTTP(pageResponse, pageRequest)
	if pageResponse.Code != http.StatusOK || !bytes.Contains(pageResponse.Body.Bytes(), []byte("古树迁移作业许可工作台")) {
		t.Fatalf("工作台入口无效: %d", pageResponse.Code)
	}
	payload, _ := json.Marshal(application.CreateCaseCommand{IdempotencyKey: "create-http-0002"})
	createRequest := httptest.NewRequest(http.MethodPost, "/api/cases", bytes.NewReader(payload))
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("创建档案失败: %d %s", createResponse.Code, createResponse.Body.String())
	}
	listRequest := httptest.NewRequest(http.MethodGet, "/api/cases", nil)
	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK || !bytes.Contains(listResponse.Body.Bytes(), []byte("GQ-")) {
		t.Fatalf("档案列表无效: %s", listResponse.Body.String())
	}
}
