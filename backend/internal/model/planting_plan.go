package model

import "time"

// PlantingPlan 种植计划实体（状态机：planned -> planting -> growing -> harvesting -> completed）。
//
// 业务约束：一块地同时只能存在一条未完成（非 completed）的种植计划。
// idx_one_active_plan_per_plot 是部分唯一索引（WHERE status <> 'completed'），
// 从数据库层兜底并发提交：事务内的行锁预检即使被绕过，重复插入也会被数据库拒绝，
// 不会留下“半条”计划。
type PlantingPlan struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	PlotID              uint       `gorm:"index;not null;uniqueIndex:idx_one_active_plan_per_plot,where:status <> 'completed'" json:"plot_id"`
	Plot                *Plot      `gorm:"foreignKey:PlotID" json:"plot"`
	UserID              uint       `gorm:"index;not null" json:"user_id"`
	User                *User      `gorm:"foreignKey:UserID" json:"user"`
	CropName            string     `gorm:"size:64;not null" json:"crop_name"`
	CropType            string     `gorm:"size:32;not null" json:"crop_type"`
	Season              string     `gorm:"size:32;not null" json:"season"`
	Status              string     `gorm:"size:32;not null;default:planned;index" json:"status"`
	PlantDate           *time.Time `json:"plant_date"`
	ExpectedHarvestDate *time.Time `json:"expected_harvest_date"`
	Notes               string     `gorm:"size:512" json:"notes"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}
