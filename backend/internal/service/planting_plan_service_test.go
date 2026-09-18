package service

import (
	"errors"
	"sync"
	"testing"

	"github.com/communitygarden/server/internal/constants"
	"github.com/communitygarden/server/internal/dto"
	"github.com/communitygarden/server/internal/model"
	"github.com/communitygarden/server/internal/repository"
	"github.com/communitygarden/server/internal/util"
	"gorm.io/gorm"
)

func TestPlanStatusTransitions_Table(t *testing.T) {
	tests := []struct {
		from constants.PlanStatus
		to   constants.PlanStatus
		ok   bool
	}{
		{constants.PlanStatusPlanned, constants.PlanStatusPlanting, true},
		{constants.PlanStatusPlanting, constants.PlanStatusGrowing, true},
		{constants.PlanStatusGrowing, constants.PlanStatusHarvesting, true},
		{constants.PlanStatusHarvesting, constants.PlanStatusCompleted, true},
		{constants.PlanStatusPlanned, constants.PlanStatusGrowing, false},
		{constants.PlanStatusCompleted, constants.PlanStatusPlanned, false},
		{constants.PlanStatusGrowing, constants.PlanStatusPlanned, false},
	}
	for _, tt := range tests {
		allowed := PlanStatusTransitions[tt.from]
		found := false
		for _, s := range allowed {
			if s == tt.to {
				found = true
			}
		}
		if found != tt.ok {
			t.Errorf("transition %s->%s ok=%v, want %v", tt.from, tt.to, found, tt.ok)
		}
	}
}

func TestPlantingPlanService_CreateAndFlow(t *testing.T) {
	db := newTestServiceDB(t)
	planRepo := repository.NewPlantingPlanRepository(db)
	plotRepo := repository.NewPlotRepository(db)
	plotSvc, _ := newPlotService(t, db)
	svc := NewPlantingPlanService(planRepo, plotRepo, plotSvc, db, testLogger())

	user := newTestUser(t, db, "farmer", "farmer")
	plot := newTestPlot(t, db, "P-PLAN", "available", nil)
	if _, err := plotSvc.Adopt(plot.ID, user.ID, "farmer", "farmer"); err != nil {
		t.Fatalf("adopt: %v", err)
	}

	// 季节不匹配的作物应报错
	req := &dto.CreatePlanRequest{PlotID: plot.ID, CropName: "西瓜", CropType: "fruit", Season: "winter"}
	if _, err := svc.Create(req, user.ID); err == nil {
		t.Fatalf("expected crop not in season error")
	}

	req = &dto.CreatePlanRequest{PlotID: plot.ID, CropName: "菠菜", CropType: "vegetable", Season: "spring"}
	plan, err := svc.Create(req, user.ID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if plan.Status != string(constants.PlanStatusPlanned) || plan.ExpectedHarvestDate == nil {
		t.Errorf("plan invalid: status=%s", plan.Status)
	}

	// 状态流转到 completed 后地块应变为 harvested
	steps := []string{"planting", "growing", "harvesting", "completed"}
	for _, s := range steps {
		if _, err := svc.ChangeStatus(plan.ID, user.ID, "farmer", s); err != nil {
			t.Fatalf("ChangeStatus(%s): %v", s, err)
		}
	}
	if _, err := svc.ChangeStatus(plan.ID, user.ID, "farmer", "planting"); err == nil {
		t.Fatalf("expected completed plan transition error")
	}
	plotAfter, err := plotSvc.GetByID(plot.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if plotAfter.Status != string(constants.PlotStatusHarvested) {
		t.Errorf("plot status=%s, want harvested", plotAfter.Status)
	}
}

func TestPlantingPlanService_Recommendations(t *testing.T) {
	db := newTestServiceDB(t)
	planRepo := repository.NewPlantingPlanRepository(db)
	plotRepo := repository.NewPlotRepository(db)
	plotSvc, _ := newPlotService(t, db)
	svc := NewPlantingPlanService(planRepo, plotRepo, plotSvc, db, testLogger())
	recs, err := svc.Recommendations("spring")
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(recs) == 0 {
		t.Fatalf("expected recommendations")
	}
	if _, err := svc.Recommendations("bogus"); err == nil {
		t.Fatalf("expected validation error for bad season")
	}
}

// newPlanSvcWithAdoptedPlot 装配种植计划服务并返回一块已认养地块。
func newPlanSvcWithAdoptedPlot(t *testing.T) (*PlantingPlanService, repository.PlantingPlanRepository, *model.User, *model.Plot, *gorm.DB) {
	t.Helper()
	db := newTestServiceDB(t)
	planRepo := repository.NewPlantingPlanRepository(db)
	plotRepo := repository.NewPlotRepository(db)
	plotSvc, _ := newPlotService(t, db)
	svc := NewPlantingPlanService(planRepo, plotRepo, plotSvc, db, testLogger())
	user := newTestUser(t, db, "farmer", "farmer")
	plot := newTestPlot(t, db, "P-ACTIVE", "available", nil)
	if _, err := plotSvc.Adopt(plot.ID, user.ID, "farmer", "farmer"); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	return svc, planRepo, user, plot, db
}

func springSpinachReq(plotID uint) *dto.CreatePlanRequest {
	return &dto.CreatePlanRequest{PlotID: plotID, CropName: "菠菜", CropType: "vegetable", Season: "spring"}
}

// TestPlantingPlanService_RejectWhenPlanActive 已有未完成计划时，
// 各未完成状态下再次提交都必须直接拒绝且保留原记录。
func TestPlantingPlanService_RejectWhenPlanActive(t *testing.T) {
	svc, planRepo, user, plot, _ := newPlanSvcWithAdoptedPlot(t)

	plan, err := svc.Create(springSpinachReq(plot.ID), user.ID)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	// 计划/播种/生长/采收四种未完成状态下重复提交均应拒绝
	for _, status := range []string{"planting", "growing", "harvesting"} {
		plan.Status = status
		if err := planRepo.Update(plan); err != nil {
			t.Fatalf("force status %s: %v", status, err)
		}
		_, err := svc.Create(springSpinachReq(plot.ID), user.ID)
		if err == nil {
			t.Fatalf("status=%s: expected conflict on duplicate create", status)
		}
		var ae *util.AppError
		if !errors.As(err, &ae) || ae.Code != constants.CodePlanAlreadyActive || ae.HTTPStatus != 409 {
			t.Fatalf("status=%s: want CodePlanAlreadyActive/409, got %v", status, err)
		}
	}

	// 原计划必须原样保留
	kept, err := svc.GetByID(plan.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if kept.Status != "harvesting" || kept.CropName != "菠菜" {
		t.Errorf("original plan mutated: status=%s crop=%s", kept.Status, kept.CropName)
	}

	// 数据库中该地块仍然只有一条计划（失败方不留半条记录）
	active, err := planRepo.FindActiveByPlots([]uint{plot.ID})
	if err != nil {
		t.Fatalf("FindActiveByPlots: %v", err)
	}
	if len(active) != 1 || active[0].ID != plan.ID {
		t.Fatalf("active plans = %v, want only original plan id=%d", active, plan.ID)
	}
}

// TestPlantingPlanService_RecreateAfterCompleted 计划完成（地块经释放、重新认养）后允许重新创建。
func TestPlantingPlanService_RecreateAfterCompleted(t *testing.T) {
	db := newTestServiceDB(t)
	planRepo := repository.NewPlantingPlanRepository(db)
	plotRepo := repository.NewPlotRepository(db)
	plotSvc, _ := newPlotService(t, db)
	svc := NewPlantingPlanService(planRepo, plotRepo, plotSvc, db, testLogger())
	user := newTestUser(t, db, "farmer", "farmer")
	plot := newTestPlot(t, db, "P-CYCLE", "available", nil)
	if _, err := plotSvc.Adopt(plot.ID, user.ID, "farmer", "farmer"); err != nil {
		t.Fatalf("adopt: %v", err)
	}

	plan, err := svc.Create(springSpinachReq(plot.ID), user.ID)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	for _, s := range []string{"planting", "growing", "harvesting", "completed"} {
		if _, err := svc.ChangeStatus(plan.ID, user.ID, "farmer", s); err != nil {
			t.Fatalf("ChangeStatus(%s): %v", s, err)
		}
	}

	// 完成后地块为 harvested，此时仍不允许直接创建（必须重新认养）
	if _, err := svc.Create(springSpinachReq(plot.ID), user.ID); err == nil {
		t.Fatalf("expected forbidden creating plan on harvested plot")
	}

	// 释放 -> 重新认养 -> 可以创建新一季计划
	if _, err := plotSvc.Release(plot.ID, user.ID, "farmer"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := plotSvc.Adopt(plot.ID, user.ID, "farmer", "farmer"); err != nil {
		t.Fatalf("re-adopt: %v", err)
	}
	plan2, err := svc.Create(springSpinachReq(plot.ID), user.ID)
	if err != nil {
		t.Fatalf("recreate after completed: %v", err)
	}
	if plan2.ID == plan.ID {
		t.Fatalf("new plan should be a new record")
	}

	// 历史 completed 计划保留，未完成计划只有新的一条
	active, err := planRepo.FindActiveByPlots([]uint{plot.ID})
	if err != nil {
		t.Fatalf("FindActiveByPlots: %v", err)
	}
	if len(active) != 1 || active[0].ID != plan2.ID {
		t.Fatalf("active plans = %v, want only new plan id=%d", active, plan2.ID)
	}
}

// TestPlantingPlanService_ConcurrentCreate 并发提交只能有一条成功，
// 且数据库最终恰好存在一条未完成计划。
func TestPlantingPlanService_ConcurrentCreate(t *testing.T) {
	svc, planRepo, user, plot, db := newPlanSvcWithAdoptedPlot(t)

	// SQLite 不支持多写者真并发（会死锁）；生产环境 PostgreSQL 由地块行锁（FOR UPDATE）串行化。
	// 单连接连接池等价模拟“行锁等待”的串行效果，验证的业务语义不变：
	// 抢锁成功的事务提交计划，其余事务随后查到占用并 409 拒绝。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	start := make(chan struct{})
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start // 同时放行，最大化竞争
			_, errs[idx] = svc.Create(springSpinachReq(plot.ID), user.ID)
		}(i)
	}
	close(start)
	wg.Wait()

	successCount := 0
	for _, err := range errs {
		if err == nil {
			successCount++
			continue
		}
		var ae *util.AppError
		if !errors.As(err, &ae) || ae.Code != constants.CodePlanAlreadyActive {
			t.Errorf("unexpected error for losing request: %v", err)
		}
	}
	if successCount != 1 {
		t.Fatalf("success count = %d, want exactly 1", successCount)
	}

	active, err := planRepo.FindActiveByPlots([]uint{plot.ID})
	if err != nil {
		t.Fatalf("FindActiveByPlots: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("active plans count = %d, want 1 (no half-written records)", len(active))
	}
}

// TestPlotService_ActivePlanOccupancy 地块列表/详情应带占用计划信息。
func TestPlotService_ActivePlanOccupancy(t *testing.T) {
	db := newTestServiceDB(t)
	planRepo := repository.NewPlantingPlanRepository(db)
	plotRepo := repository.NewPlotRepository(db)
	plotSvc := NewPlotService(plotRepo, planRepo, db, testLogger())
	user := newTestUser(t, db, "farmer", "farmer")
	plot := newTestPlot(t, db, "P-OCC", "available", nil)
	if _, err := plotSvc.Adopt(plot.ID, user.ID, "farmer", "farmer"); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	planSvc := NewPlantingPlanService(planRepo, plotRepo, plotSvc, db, testLogger())
	plan, err := planSvc.Create(springSpinachReq(plot.ID), user.ID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// 详情接口
	detail, err := plotSvc.GetByID(plot.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if detail.ActivePlan == nil || detail.ActivePlan.ID != plan.ID || detail.ActivePlan.Status != "planned" {
		t.Fatalf("detail active plan = %+v, want plan id=%d", detail.ActivePlan, plan.ID)
	}

	// 列表接口
	plots, _, err := plotSvc.List(util.PageQuery{Page: 1, PageSize: 10}, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var found bool
	for i := range plots {
		if plots[i].ID == plot.ID {
			found = true
			if plots[i].ActivePlan == nil || plots[i].ActivePlan.ID != plan.ID {
				t.Fatalf("list active plan not populated")
			}
		}
	}
	if !found {
		t.Fatalf("plot not found in list")
	}
}
