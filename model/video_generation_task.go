package model

import (
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/random"
)

type VideoGenerationTask struct {
	Id             string `json:"id" gorm:"primaryKey;type:varchar(64)"`
	UserId         int    `json:"user_id" gorm:"index"`
	TokenId        int    `json:"token_id" gorm:"index"`
	ChannelId      int    `json:"channel_id" gorm:"index"`
	Provider       string `json:"provider" gorm:"type:varchar(32);index"`
	Model          string `json:"model" gorm:"type:varchar(128);index"`
	ProviderTaskId string `json:"provider_task_id" gorm:"type:varchar(128);uniqueIndex"`
	Status         string `json:"status" gorm:"type:varchar(32);index"`
	RequestBody    string `json:"request_body" gorm:"type:text"`
	ResponseBody   string `json:"response_body" gorm:"type:text"`
	ResultURL      string `json:"result_url" gorm:"type:text"`
	ErrorCode      string `json:"error_code" gorm:"type:varchar(128)"`
	ErrorMessage   string `json:"error_message" gorm:"type:text"`
	Quota          int64  `json:"quota" gorm:"bigint;default:0"`
	IsBilled       bool   `json:"is_billed" gorm:"default:false"`
	CreatedTime    int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime    int64  `json:"updated_time" gorm:"bigint"`
}

func (task *VideoGenerationTask) Insert() error {
	now := helper.GetTimestamp()
	if task.Id == "" {
		task.Id = random.GetUUID()
	}
	task.CreatedTime = now
	task.UpdatedTime = now
	return DB.Create(task).Error
}

func (task *VideoGenerationTask) Update() error {
	task.UpdatedTime = helper.GetTimestamp()
	return DB.Model(task).Where("id = ?", task.Id).Updates(task).Error
}

func GetUserVideoGenerationTaskById(id string, userId int) (*VideoGenerationTask, error) {
	var task VideoGenerationTask
	err := DB.First(&task, "id = ? AND user_id = ?", id, userId).Error
	return &task, err
}
