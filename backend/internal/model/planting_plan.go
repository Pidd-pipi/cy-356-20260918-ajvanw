package model

import "time"

// PlantingPlan 种植计划实体（状态机：planned -> planting -> growing -> harvesting -> completed）。
// uniq_plans_active_plot 部分唯一索引：一块地同一时间只允许一条未完成（非 completed）计划，
// 并发提交由数据库兜底只成功一条，失败方事务回滚不留半条计划。
type PlantingPlan struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	PlotID             uint       `gorm:"index;not null;uniqueIndex:uniq_plans_active_plot,where:status <> 'completed'" json:"plot_id"`
	Plot               *Plot      `gorm:"foreignKey:PlotID" json:"plot"`
	UserID             uint       `gorm:"index;not null" json:"user_id"`
	User               *User      `gorm:"foreignKey:UserID" json:"user"`
	CropName           string     `gorm:"size:64;not null" json:"crop_name"`
	CropType           string     `gorm:"size:32;not null" json:"crop_type"`
	Season             string     `gorm:"size:32;not null" json:"season"`
	Status             string     `gorm:"size:32;not null;default:planned;index" json:"status"`
	PlantDate          *time.Time `json:"plant_date"`
	ExpectedHarvestDate *time.Time `json:"expected_harvest_date"`
	Notes              string     `gorm:"size:512" json:"notes"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}
