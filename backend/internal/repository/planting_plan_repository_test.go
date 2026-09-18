package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/communitygarden/server/internal/model"
	"github.com/communitygarden/server/internal/util"
)

func TestPlantingPlanRepository_ListAndCount(t *testing.T) {
	db := newTestDB(t)
	repo := NewPlantingPlanRepository(db)
	user := seedUser(t, db, "farmer", "farmer")
	plot := &model.Plot{Name: "P", Code: "P-100", Area: 10, SoilType: "loam", Sunlight: "full", Latitude: 31.0, Longitude: 121.0, Status: "adopted", AdopterID: &user.ID}
	plot2 := &model.Plot{Name: "P2", Code: "P-101", Area: 10, SoilType: "loam", Sunlight: "full", Latitude: 31.0, Longitude: 121.0, Status: "adopted", AdopterID: &user.ID}
	if err := db.Create(plot).Error; err != nil {
		t.Fatalf("create plot: %v", err)
	}
	if err := db.Create(plot2).Error; err != nil {
		t.Fatalf("create plot2: %v", err)
	}
	now := time.Now()
	// 同一地块只允许一条未完成计划：planned 放 plot、growing 放 plot2，completed 留在 plot
	plans := []struct {
		plotID uint
		status string
		crop   string
	}{
		{plot.ID, "planned", "菠菜"}, {plot2.ID, "growing", "番茄"}, {plot.ID, "completed", "白菜"},
	}
	for _, p := range plans {
		plan := &model.PlantingPlan{PlotID: p.plotID, UserID: user.ID, CropName: p.crop, CropType: "vegetable", Season: "spring", Status: p.status, PlantDate: &now, ExpectedHarvestDate: &now}
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

func TestPlantingPlanRepository_ActivePlanOccupancy(t *testing.T) {
	db := newTestDB(t)
	repo := NewPlantingPlanRepository(db)
	user := seedUser(t, db, "farmer2", "farmer")
	plot := &model.Plot{Name: "P2", Code: "P-200", Area: 10, SoilType: "loam", Sunlight: "full", Latitude: 31.0, Longitude: 121.0, Status: "adopted", AdopterID: &user.ID}
	otherPlot := &model.Plot{Name: "P3", Code: "P-300", Area: 10, SoilType: "loam", Sunlight: "full", Latitude: 31.0, Longitude: 121.0, Status: "available"}
	if err := db.Create(plot).Error; err != nil {
		t.Fatalf("create plot: %v", err)
	}
	if err := db.Create(otherPlot).Error; err != nil {
		t.Fatalf("create other plot: %v", err)
	}
	now := time.Now()
	mkPlan := func(status string, crop string) *model.PlantingPlan {
		return &model.PlantingPlan{PlotID: plot.ID, UserID: user.ID, CropName: crop, CropType: "vegetable", Season: "spring", Status: status, PlantDate: &now, ExpectedHarvestDate: &now}
	}
	completed := mkPlan("completed", "白菜")
	if err := repo.Create(completed); err != nil {
		t.Fatalf("create completed plan: %v", err)
	}
	active := mkPlan("growing", "番茄")
	if err := repo.Create(active); err != nil {
		t.Fatalf("create active plan: %v", err)
	}

	// 单地块事务内查询：completed 不算占用，未完成计划能查到
	got, err := repo.FindActiveByPlotForUpdate(db, plot.ID)
	if err != nil {
		t.Fatalf("FindActiveByPlotForUpdate: %v", err)
	}
	if got.ID != active.ID {
		t.Errorf("active plan id=%d, want %d", got.ID, active.ID)
	}
	if _, err := repo.FindActiveByPlotForUpdate(db, otherPlot.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("empty plot: want ErrNotFound, got %v", err)
	}

	// 批量查询：只返回未完成计划
	list, err := repo.FindActiveByPlots([]uint{plot.ID, otherPlot.ID})
	if err != nil {
		t.Fatalf("FindActiveByPlots: %v", err)
	}
	if len(list) != 1 || list[0].ID != active.ID {
		t.Errorf("batch active = %v, want only plan id=%d", list, active.ID)
	}
	if list, err := repo.FindActiveByPlots(nil); err != nil || len(list) != 0 {
		t.Errorf("empty ids: list=%v err=%v", list, err)
	}
}

func TestPlantingPlanRepository_PartialUniqueIndexRejectsDuplicate(t *testing.T) {
	db := newTestDB(t)
	repo := NewPlantingPlanRepository(db)
	user := seedUser(t, db, "farmer3", "farmer")
	plot := &model.Plot{Name: "P4", Code: "P-400", Area: 10, SoilType: "loam", Sunlight: "full", Latitude: 31.0, Longitude: 121.0, Status: "adopted", AdopterID: &user.ID}
	if err := db.Create(plot).Error; err != nil {
		t.Fatalf("create plot: %v", err)
	}
	now := time.Now()
	first := &model.PlantingPlan{PlotID: plot.ID, UserID: user.ID, CropName: "菠菜", CropType: "vegetable", Season: "spring", Status: "planned", PlantDate: &now, ExpectedHarvestDate: &now}
	if err := repo.Create(first); err != nil {
		t.Fatalf("first create: %v", err)
	}
	second := &model.PlantingPlan{PlotID: plot.ID, UserID: user.ID, CropName: "番茄", CropType: "vegetable", Season: "summer", Status: "growing", PlantDate: &now, ExpectedHarvestDate: &now}
	err := repo.Create(second)
	if err == nil {
		t.Fatalf("expected unique violation, insert succeeded")
	}
	if !IsDuplicateKeyErr(err) {
		t.Fatalf("IsDuplicateKeyErr=false for err=%v", err)
	}

	// 完成历史计划后，允许为同一地块新建计划
	if err := db.Model(&model.PlantingPlan{}).Where("id = ?", first.ID).Update("status", "completed").Error; err != nil {
		t.Fatalf("complete plan: %v", err)
	}
	third := &model.PlantingPlan{PlotID: plot.ID, UserID: user.ID, CropName: "白菜", CropType: "vegetable", Season: "autumn", Status: "planned", PlantDate: &now, ExpectedHarvestDate: &now}
	if err := repo.Create(third); err != nil {
		t.Fatalf("create after completion: %v", err)
	}
}
