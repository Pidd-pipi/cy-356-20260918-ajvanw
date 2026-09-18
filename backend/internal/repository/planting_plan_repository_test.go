package repository

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/communitygarden/server/internal/model"
	"github.com/communitygarden/server/internal/util"
)

func TestPlantingPlanRepository_ListAndCount(t *testing.T) {
	db := newTestDB(t)
	repo := NewPlantingPlanRepository(db)
	user := seedUser(t, db, "farmer", "farmer")
	now := time.Now()
	plans := []struct {
		status string
		crop   string
	}{
		{"planned", "菠菜"}, {"growing", "番茄"}, {"completed", "白菜"},
	}
	// 一块地只允许一条未完成计划（uniq_plans_active_plot 部分唯一索引），每条计划使用独立地块
	for i, p := range plans {
		plot := &model.Plot{Name: "P", Code: fmt.Sprintf("P-10%d", i), Area: 10, SoilType: "loam", Sunlight: "full", Latitude: 31.0, Longitude: 121.0, Status: "adopted", AdopterID: &user.ID}
		if err := db.Create(plot).Error; err != nil {
			t.Fatalf("create plot: %v", err)
		}
		plan := &model.PlantingPlan{PlotID: plot.ID, UserID: user.ID, CropName: p.crop, CropType: "vegetable", Season: "spring", Status: p.status, PlantDate: &now, ExpectedHarvestDate: &now}
		if err := repo.Create(plan); err != nil {
			t.Fatalf("create plan: %v", err)
		}
	}
	list, total, err := repo.List(util.PageQuery{Page: 1, PageSize: 10}, user.ID, "")
	if err != nil || total != 3 || len(list) != 3 {
		t.Errorf("list: total=%d len=%d err=%v", total, len(list), err)
	}
	cnt, err := repo.CountByUser(user.ID)
	if err != nil || cnt != 3 {
		t.Errorf("CountByUser=%d err=%v", cnt, err)
	}
	active, err := repo.CountActiveByUser(user.ID)
	if err != nil || active != 2 {
		t.Errorf("CountActiveByUser=%d err=%v", active, err)
	}
	counts, err := repo.CountByStatus()
	if err != nil {
		t.Fatalf("CountByStatus: %v", err)
	}
	if counts["planned"] != 1 || counts["growing"] != 1 || counts["completed"] != 1 {
		t.Errorf("counts = %v", counts)
	}
}

func TestPlantingPlanRepository_FindActiveByPlotID(t *testing.T) {
	db := newTestDB(t)
	repo := NewPlantingPlanRepository(db)
	user := seedUser(t, db, "farmer", "farmer")
	newPlot := func(code string) *model.Plot {
		p := &model.Plot{Name: code, Code: code, Area: 10, SoilType: "loam", Sunlight: "full", Latitude: 31.0, Longitude: 121.0, Status: "adopted", AdopterID: &user.ID}
		if err := db.Create(p).Error; err != nil {
			t.Fatalf("create plot: %v", err)
		}
		return p
	}
	newPlan := func(plotID uint, status string) *model.PlantingPlan {
		p := &model.PlantingPlan{PlotID: plotID, UserID: user.ID, CropName: "菠菜", CropType: "vegetable", Season: "spring", Status: status}
		if err := repo.Create(p); err != nil {
			t.Fatalf("create plan: %v", err)
		}
		return p
	}

	// 无计划的地块：ErrNotFound
	emptyPlot := newPlot("P-EMPTY")
	if _, err := repo.FindActiveByPlotID(db, emptyPlot.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("empty plot err=%v, want ErrNotFound", err)
	}

	// 仅有已完成计划的地块：ErrNotFound（完成后允许重新创建）
	donePlot := newPlot("P-DONE")
	newPlan(donePlot.ID, "completed")
	if _, err := repo.FindActiveByPlotID(db, donePlot.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("completed-only plot err=%v, want ErrNotFound", err)
	}

	// 有未完成计划的地块：返回占用中的计划
	busyPlot := newPlot("P-BUSY")
	active := newPlan(busyPlot.ID, "growing")
	got, err := repo.FindActiveByPlotID(db, busyPlot.ID)
	if err != nil {
		t.Fatalf("FindActiveByPlotID: %v", err)
	}
	if got.ID != active.ID || got.Status != "growing" {
		t.Errorf("got id=%d status=%s, want id=%d growing", got.ID, got.Status, active.ID)
	}

	// 部分唯一索引兜底：同一地块插入第二条未完成计划必须失败
	dup := &model.PlantingPlan{PlotID: busyPlot.ID, UserID: user.ID, CropName: "生菜", CropType: "vegetable", Season: "spring", Status: "planned"}
	if err := repo.Create(dup); err == nil {
		t.Fatalf("expected uniq_plans_active_plot violation for second active plan")
	}
	// 已完成计划不占用索引：同一地块可再插入 completed
	done2 := &model.PlantingPlan{PlotID: busyPlot.ID, UserID: user.ID, CropName: "白菜", CropType: "vegetable", Season: "autumn", Status: "completed"}
	if err := repo.Create(done2); err != nil {
		t.Fatalf("completed plan on busy plot should be allowed: %v", err)
	}
}
