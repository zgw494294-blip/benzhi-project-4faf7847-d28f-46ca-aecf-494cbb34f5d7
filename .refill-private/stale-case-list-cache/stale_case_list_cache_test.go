package stale_case_list_cache_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"heritage-tree-relocation-permit/internal/application"
	"heritage-tree-relocation-permit/internal/domain"
	"heritage-tree-relocation-permit/internal/storage/journal"
	"heritage-tree-relocation-permit/internal/web"
)

func TestCaseListRefreshesAfterCommittedAssessment(t *testing.T) {
	repository, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	service := application.NewService(repository, application.RealClock{}, nil)
	handler := web.NewHandler(service, http.NotFoundHandler())
	created := domain.RelocationCase{}
	requestJSON(t, handler, http.MethodPost, "/api/cases", application.CreateCaseCommand{
		IdempotencyKey: "refill-create-0001",
	}, http.StatusCreated, &created)

	warmed := struct {
		Items []domain.RelocationCase `json:"items"`
	}{}
	requestJSON(t, handler, http.MethodGet, "/api/cases", nil, http.StatusOK, &warmed)
	if len(warmed.Items) != 1 || warmed.Items[0].Version != 1 || warmed.Items[0].Status != domain.StatusDraft {
		t.Fatalf("列表预热状态不符合前置条件: %#v", warmed.Items)
	}

	assessedAt := time.Now().UTC().Add(-time.Hour)
	command := application.AssessmentCommand{
		ExpectedVersion: created.Version,
		IdempotencyKey:  "refill-assess-0001",
		Tree: domain.TreeProfile{
			SpeciesName: "香樟", ProtectionGrade: "二级", TrunkDiameterCM: 50,
			CrownRadiusM: 3, HealthGrade: "良好", RootSurvey: "根系完整并已完成定位",
			AssessedAt: assessedAt, Assessor: "树体评估员",
		},
		Destination: domain.DestinationAssessment{
			SiteName: "植物园东区", AvailableRadiusM: 5, SoilType: "壤土", SoilPH: 6.5,
			DrainageGrade: "良好", RouteClearanceM: 3, AssessedAt: assessedAt, Assessor: "场地评估员",
		},
	}
	updated := domain.RelocationCase{}
	requestJSON(t, handler, http.MethodPost, fmt.Sprintf("/api/cases/%s/assessments", created.CaseID), command, http.StatusOK, &updated)
	if updated.Version != 2 || updated.Status != domain.StatusAssessed {
		t.Fatalf("评估写入未成功: version=%d status=%s", updated.Version, updated.Status)
	}
	stored, err := service.GetCase(context.Background(), created.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Version != updated.Version || stored.Status != updated.Status {
		t.Fatalf("journal 未保存评估结果: version=%d status=%s", stored.Version, stored.Status)
	}

	listed := struct {
		Items []domain.RelocationCase `json:"items"`
	}{}
	requestJSON(t, handler, http.MethodGet, "/api/cases", nil, http.StatusOK, &listed)
	if len(listed.Items) != 1 {
		t.Fatalf("期望列表包含一个档案，实际 %d", len(listed.Items))
	}
	if listed.Items[0].Version != stored.Version || listed.Items[0].Status != stored.Status {
		t.Fatalf("写入提交后列表仍返回旧聚合: listVersion=%d listStatus=%s storedVersion=%d storedStatus=%s", listed.Items[0].Version, listed.Items[0].Status, stored.Version, stored.Status)
	}
}

func requestJSON(t *testing.T, handler http.Handler, method, path string, input any, wantStatus int, output any) {
	t.Helper()
	var body bytes.Buffer
	if input != nil {
		if err := json.NewEncoder(&body).Encode(input); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &body)
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != wantStatus {
		t.Fatalf("%s %s 返回 %d，期望 %d: %s", method, path, response.Code, wantStatus, response.Body.String())
	}
	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		t.Fatalf("解析 %s %s 响应失败: %v", method, path, err)
	}
}
