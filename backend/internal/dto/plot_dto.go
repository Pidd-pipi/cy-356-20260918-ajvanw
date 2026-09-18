package dto

import (
	"github.com/communitygarden/server/internal/model"
	"github.com/communitygarden/server/internal/util"
)

// CreatePlotRequest 创建地块（管理员）。
type CreatePlotRequest struct {
	Name        string  `json:"name" binding:"required,max=128"`
	Code        string  `json:"code" binding:"required,max=32"`
	Area        float64 `json:"area" binding:"required,gt=0"`
	SoilType    string  `json:"soil_type" binding:"required,oneof=loam clay sand black"`
	Sunlight    string  `json:"sunlight" binding:"required,oneof=full partial shade"`
	Latitude    float64 `json:"latitude" binding:"required,min=-90,max=90"`
	Longitude   float64 `json:"longitude" binding:"required,min=-180,max=180"`
	Description string  `json:"description" binding:"omitempty,max=512"`
}

// UpdatePlotRequest 更新地块（管理员）。
type UpdatePlotRequest struct {
	Name        *string  `json:"name" binding:"omitempty,max=128"`
	Code        *string  `json:"code" binding:"omitempty,max=32"`
	Area        *float64 `json:"area" binding:"omitempty,gt=0"`
	SoilType    *string  `json:"soil_type" binding:"omitempty,oneof=loam clay sand black"`
	Sunlight    *string  `json:"sunlight" binding:"omitempty,oneof=full partial shade"`
	Latitude    *float64 `json:"latitude" binding:"omitempty,min=-90,max=90"`
	Longitude   *float64 `json:"longitude" binding:"omitempty,min=-180,max=180"`
	Description *string  `json:"description" binding:"omitempty,max=512"`
}

// ActivePlanInfo 地块当前未完成种植计划的占用信息（制定计划窗口据此禁用地块并展示占用原因）。
type ActivePlanInfo struct {
	ID         uint   `json:"id"`
	CropName   string `json:"crop_name"`
	Status     string `json:"status"`
	StatusText string `json:"status_text"`
	UserID     uint   `json:"user_id"`
	Username   string `json:"username"`
}

// PlotOutDTO 地块输出。
type PlotOutDTO struct {
	ID          uint            `json:"id"`
	Name        string          `json:"name"`
	Code        string          `json:"code"`
	Area        float64         `json:"area"`
	SoilType    string          `json:"soil_type"`
	Sunlight    string          `json:"sunlight"`
	Latitude    float64         `json:"latitude"`
	Longitude   float64         `json:"longitude"`
	Status      string          `json:"status"`
	AdopterID   *uint           `json:"adopter_id"`
	Adopter     *UserOutDTO     `json:"adopter"`
	Description string          `json:"description"`
	ActivePlan  *ActivePlanInfo `json:"active_plan"`
	CreatedAt   string          `json:"created_at"`
}

// ToPlotOutDTO 模型转 DTO。
func ToPlotOutDTO(p *model.Plot) *PlotOutDTO {
	dto := &PlotOutDTO{
		ID:          p.ID,
		Name:        p.Name,
		Code:        p.Code,
		Area:        p.Area,
		SoilType:    p.SoilType,
		Sunlight:    p.Sunlight,
		Latitude:    p.Latitude,
		Longitude:   p.Longitude,
		Status:      p.Status,
		AdopterID:   p.AdopterID,
		Description: p.Description,
		CreatedAt:   p.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	if p.Adopter != nil {
		dto.Adopter = ToUserOutDTO(p.Adopter)
	}
	if p.ActivePlan != nil {
		info := &ActivePlanInfo{
			ID:         p.ActivePlan.ID,
			CropName:   p.ActivePlan.CropName,
			Status:     p.ActivePlan.Status,
			StatusText: util.PlanStatusText(p.ActivePlan.Status),
			UserID:     p.ActivePlan.UserID,
		}
		if p.ActivePlan.User != nil {
			info.Username = p.ActivePlan.User.Username
		}
		dto.ActivePlan = info
	}
	return dto
}
