package service

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/communitygarden/server/internal/constants"
	"github.com/communitygarden/server/internal/dto"
	"github.com/communitygarden/server/internal/model"
	"github.com/communitygarden/server/internal/repository"
	"github.com/communitygarden/server/internal/util"
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

// 一块地只能有一条未完成计划：已有计划处于计划/播种/生长/采收状态时，再次提交直接拒绝并保留原记录。
func TestPlantingPlanService_CreateRejectsActivePlan(t *testing.T) {
	for _, status := range constants.ActivePlanStatuses {
		t.Run(string(status), func(t *testing.T) {
			db := newTestServiceDB(t)
			planRepo := repository.NewPlantingPlanRepository(db)
			plotRepo := repository.NewPlotRepository(db)
			plotSvc, _ := newPlotService(t, db)
			svc := NewPlantingPlanService(planRepo, plotRepo, plotSvc, db, testLogger())

			user := newTestUser(t, db, "farmer-"+string(status), "farmer")
			plot := newTestPlot(t, db, "P-"+string(status), "available", nil)
			if _, err := plotSvc.Adopt(plot.ID, user.ID, "farmer", "farmer"); err != nil {
				t.Fatalf("adopt: %v", err)
			}
			req := &dto.CreatePlanRequest{PlotID: plot.ID, CropName: "菠菜", CropType: "vegetable", Season: "spring"}
			plan, err := svc.Create(req, user.ID)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			// 将已有计划推进到被测占用状态
			if err := db.Model(&model.PlantingPlan{}).Where("id = ?", plan.ID).Update("status", string(status)).Error; err != nil {
				t.Fatalf("update status: %v", err)
			}

			_, err = svc.Create(&dto.CreatePlanRequest{PlotID: plot.ID, CropName: "生菜", CropType: "vegetable", Season: "spring"}, user.ID)
			var appErr *util.AppError
			if !errors.As(err, &appErr) || appErr.Code != constants.CodePlanActiveExists {
				t.Fatalf("err=%v, want AppError code=%d", err, constants.CodePlanActiveExists)
			}
			if appErr.HTTPStatus != 409 {
				t.Errorf("http status=%d, want 409", appErr.HTTPStatus)
			}

			// 原记录必须保留：状态不变，且该地块仍只有一条计划（失败方不留半条计划）
			kept, err := svc.GetByID(plan.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if kept.Status != string(status) || kept.CropName != "菠菜" {
				t.Errorf("original plan mutated: status=%s crop=%s", kept.Status, kept.CropName)
			}
			var count int64
			if err := db.Model(&model.PlantingPlan{}).Where("plot_id = ?", plot.ID).Count(&count).Error; err != nil {
				t.Fatalf("count: %v", err)
			}
			if count != 1 {
				t.Errorf("plans for plot=%d, want 1", count)
			}
		})
	}
}

// 计划完成后才允许重新创建（按地块生命周期：完成 → 待释放 → 释放 → 重新认养 → 再创建）。
func TestPlantingPlanService_CreateAllowedAfterCompleted(t *testing.T) {
	db := newTestServiceDB(t)
	planRepo := repository.NewPlantingPlanRepository(db)
	plotRepo := repository.NewPlotRepository(db)
	plotSvc, _ := newPlotService(t, db)
	svc := NewPlantingPlanService(planRepo, plotRepo, plotSvc, db, testLogger())

	user := newTestUser(t, db, "farmer-done", "farmer")
	plot := newTestPlot(t, db, "P-DONE", "available", nil)
	if _, err := plotSvc.Adopt(plot.ID, user.ID, "farmer", "farmer"); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	req := &dto.CreatePlanRequest{PlotID: plot.ID, CropName: "菠菜", CropType: "vegetable", Season: "spring"}
	plan, err := svc.Create(req, user.ID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, s := range []string{"planting", "growing", "harvesting", "completed"} {
		if _, err := svc.ChangeStatus(plan.ID, user.ID, "farmer", s); err != nil {
			t.Fatalf("ChangeStatus(%s): %v", s, err)
		}
	}
	// 完成后地块进入待释放（harvested），释放并重新认养后允许再次创建
	if _, err := plotSvc.Release(plot.ID, user.ID, "farmer"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := plotSvc.Adopt(plot.ID, user.ID, "farmer", "farmer"); err != nil {
		t.Fatalf("re-adopt: %v", err)
	}
	again, err := svc.Create(&dto.CreatePlanRequest{PlotID: plot.ID, CropName: "生菜", CropType: "vegetable", Season: "spring"}, user.ID)
	if err != nil {
		t.Fatalf("Create after completed: %v", err)
	}
	if again.ID == plan.ID {
		t.Errorf("expected a new plan row, got id=%d", again.ID)
	}
	var count int64
	if err := db.Model(&model.PlantingPlan{}).Where("plot_id = ?", plot.ID).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("plans for plot=%d, want 2（1 条已完成 + 1 条新计划）", count)
	}
}

// 并发提交只能有一条成功，失败一方不能留下半条计划。
// 测试库为内存 SQLite，用单连接串行化并发事务，等价于行锁下后提交者读到先提交者的活跃计划。
func TestPlantingPlanService_CreateConcurrentSingleSuccess(t *testing.T) {
	db := newTestServiceDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	planRepo := repository.NewPlantingPlanRepository(db)
	plotRepo := repository.NewPlotRepository(db)
	plotSvc, _ := newPlotService(t, db)
	svc := NewPlantingPlanService(planRepo, plotRepo, plotSvc, db, testLogger())

	user := newTestUser(t, db, "farmer-race", "farmer")
	plot := newTestPlot(t, db, "P-RACE", "available", nil)
	if _, err := plotSvc.Adopt(plot.ID, user.ID, "farmer", "farmer"); err != nil {
		t.Fatalf("adopt: %v", err)
	}

	const n = 5
	var wg sync.WaitGroup
	var success int64
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Create(&dto.CreatePlanRequest{PlotID: plot.ID, CropName: "菠菜", CropType: "vegetable", Season: "spring"}, user.ID)
			if err == nil {
				atomic.AddInt64(&success, 1)
				return
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	if success != 1 {
		t.Errorf("concurrent creates succeeded=%d, want exactly 1", success)
	}
	for err := range errs {
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodePlanActiveExists {
			t.Errorf("rejected err=%v, want AppError code=%d", err, constants.CodePlanActiveExists)
		}
	}
	var count int64
	if err := db.Model(&model.PlantingPlan{}).Where("plot_id = ?", plot.ID).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("plans for plot=%d, want 1（失败方不能留下半条计划）", count)
	}
}
