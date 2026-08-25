package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"heritage-tree-relocation-permit/internal/application"
	"heritage-tree-relocation-permit/internal/domain"
)

type smokeClient struct {
	baseURL string
	client  *http.Client
}

func runSelfcheck(ctx context.Context, baseURL string) error {
	smoke := smokeClient{baseURL: baseURL, client: &http.Client{Timeout: 3 * time.Second}}
	if err := smoke.waitHealthy(ctx); err != nil {
		return err
	}
	var current domain.RelocationCase
	if err := smoke.post(ctx, "/api/cases", application.CreateCaseCommand{IdempotencyKey: "smoke-create-0001"}, &current, http.StatusCreated); err != nil {
		return err
	}
	now := time.Now().UTC().Add(-24 * time.Hour)
	assessment := application.AssessmentCommand{ExpectedVersion: current.Version, IdempotencyKey: "smoke-assessment-0001", Tree: domain.TreeProfile{SpeciesName: "香樟", ProtectionGrade: "二级", TrunkDiameterCM: 60, CrownRadiusM: 4, HealthGrade: "一般", RootSurvey: "主根完整，东侧侧根需要分段保护", AssessedAt: now, Assessor: "树体评估员"}, Destination: domain.DestinationAssessment{SiteName: "城市植物园东区", AvailableRadiusM: 6, SoilType: "壤土", SoilPH: 6.8, DrainageGrade: "可改良", RouteClearanceM: 3.2, AssessedAt: now, Assessor: "场地评估员"}}
	if err := smoke.post(ctx, casePath(current, "/assessments"), assessment, &current, http.StatusOK); err != nil {
		return err
	}
	revision := application.RevisionCommand{ExpectedVersion: current.Version, IdempotencyKey: "smoke-revision-0001", RootBallDiameterCM: 500, ExcavationMeasures: "分区断根并保持根系湿润", PackingMeasures: "麻布与钢丝网分层固定土球", TransportMeasures: "低速运输并安排引导车辆", PlantingMeasures: "分层回填并设置透气管", AftercareMeasures: "两年水肥与树势监测", RiskControls: map[string]string{"tree_health": "设置树冠喷雾并缩短离土时间", "drainage_improvement": "增设盲沟并换填种植土", "route_clearance": "转弯点铺设钢板并专人指挥"}}
	if err := smoke.post(ctx, casePath(current, "/revisions"), revision, &current, http.StatusOK); err != nil {
		return err
	}
	review := application.ReviewCommand{ExpectedVersion: current.Version, IdempotencyKey: "smoke-review-0001", Findings: []application.FindingInput{{Severity: domain.SeverityBlocking, Category: "运输组织", Description: "转弯点指挥人员职责需要明确"}}}
	if err := smoke.post(ctx, casePath(current, "/reviews"), review, &current, http.StatusOK); err != nil {
		return err
	}
	if len(current.Findings) != 1 || current.Status != domain.StatusCorrectionRequired {
		return fmt.Errorf("技术审查未生成预期阻断问题")
	}
	findingPath := fmt.Sprintf("/api/cases/%s/findings/%s", current.CaseID, current.Findings[0].FindingID)
	evidence := application.EvidenceCommand{ExpectedVersion: current.Version, IdempotencyKey: "smoke-evidence-0001", Evidence: "已补充岗位职责表和现场签字记录", Submitter: "方案编制人员"}
	if err := smoke.post(ctx, findingPath+"/evidence", evidence, &current, http.StatusOK); err != nil {
		return err
	}
	verify := application.VerifyFindingCommand{ExpectedVersion: current.Version, IdempotencyKey: "smoke-verify-0001", Reviewer: "技术审查员", Opinion: "整改材料与现场记录一致", Accepted: true}
	if err := smoke.post(ctx, findingPath+"/verify", verify, &current, http.StatusOK); err != nil {
		return err
	}
	var checklist domain.FrozenChecklist
	if err := smoke.get(ctx, casePath(current, "/precheck-checklist"), &checklist); err != nil {
		return err
	}
	items := append([]domain.ChecklistItem(nil), checklist.Items...)
	for i := range items {
		items[i].Passed = true
		items[i].Note = "符合开工条件"
	}
	precheck := application.PrecheckCommand{ExpectedVersion: current.Version, IdempotencyKey: "smoke-precheck-0001", ChecklistID: checklist.ChecklistID, Inspector: "现场核验员", CheckedAt: time.Now().UTC(), Items: items}
	if err := smoke.post(ctx, casePath(current, "/prechecks"), precheck, &current, http.StatusOK); err != nil {
		return err
	}
	var eligibility domain.PermitEligibility
	if err := smoke.get(ctx, casePath(current, "/permit-eligibility"), &eligibility); err != nil {
		return err
	}
	if !eligibility.Eligible || eligibility.Version != current.Version {
		return fmt.Errorf("许可签发资格预检未全部通过")
	}
	permit := application.IssuePermitCommand{ExpectedVersion: current.Version, IdempotencyKey: "smoke-permit-0001", IssuedBy: "城市树木保护专员"}
	if err := smoke.post(ctx, casePath(current, "/permit"), permit, &current, http.StatusCreated); err != nil {
		return err
	}
	if current.Status != domain.StatusApproved || current.Permit == nil || len(current.Permit.ContentDigest) != 64 {
		return fmt.Errorf("许可签发结果不完整")
	}
	var fetched application.PermitView
	if err := smoke.get(ctx, casePath(current, "/permit"), &fetched); err != nil {
		return err
	}
	if fetched.ContentDigest != current.Permit.ContentDigest || fetched.DigestVerification.Status != "matching" || fetched.Snapshot.TreeProfile == nil || fetched.Snapshot.Destination == nil || fetched.Snapshot.Revision == nil || fetched.Snapshot.Checklist == nil || fetched.Snapshot.Precheck == nil {
		return fmt.Errorf("许可查询的冻结快照或摘要复验结果不完整")
	}
	var timeline struct {
		Items []application.TimelineEntry `json:"items"`
	}
	if err := smoke.get(ctx, casePath(current, "/timeline"), &timeline); err != nil {
		return err
	}
	if len(timeline.Items) != 8 {
		return fmt.Errorf("时间线事件数不正确：期望 8，实际 %d", len(timeline.Items))
	}
	return nil
}

func casePath(c domain.RelocationCase, suffix string) string {
	return "/api/cases/" + c.CaseID + suffix
}

func (s smokeClient) waitHealthy(ctx context.Context) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		var response map[string]any
		if err := s.get(ctx, "/api/health", &response); err == nil && response["status"] == "ok" {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待健康检查超时: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (s smokeClient) post(ctx context.Context, path string, input, output any, expectedStatus int) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return s.do(request, output, expectedStatus)
}

func (s smokeClient) get(ctx context.Context, path string, output any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+path, nil)
	if err != nil {
		return err
	}
	return s.do(request, output, http.StatusOK)
}

func (s smokeClient) do(request *http.Request, output any, expectedStatus int) error {
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("请求 %s 失败: %w", request.URL.Path, err)
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("请求 %s 返回 %d，期望 %d: %s", request.URL.Path, response.StatusCode, expectedStatus, body)
	}
	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		return fmt.Errorf("解析 %s 响应失败: %w", request.URL.Path, err)
	}
	return nil
}
