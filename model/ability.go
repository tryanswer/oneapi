package model

import (
	"context"
	"hash/fnv"
	"math/rand"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/utils"
)

type Ability struct {
	Group     string `json:"group" gorm:"type:varchar(32);primaryKey;autoIncrement:false"`
	Model     string `json:"model" gorm:"primaryKey;autoIncrement:false"`
	ChannelId int    `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
	Enabled   bool   `json:"enabled"`
	Priority  *int64 `json:"priority" gorm:"bigint;default:0;index"`
}

func GetRandomSatisfiedChannel(group string, model string, ignoreFirstPriority bool, seed string) (*Channel, error) {
	ability := Ability{}
	groupCol := "`group`"
	trueVal := "1"
	if common.UsingPostgreSQL {
		groupCol = `"group"`
		trueVal = "true"
	}

	var err error
	var maxPriority int64
	err = DB.Model(&Ability{}).Select("MAX(priority)").Where(groupCol+" = ? and model = ? and enabled = "+trueVal, group, model).Scan(&maxPriority).Error
	if err != nil {
		return nil, err
	}

	var abilities []Ability
	if ignoreFirstPriority {
		err = DB.Where(groupCol+" = ? and model = ? and enabled = "+trueVal+" and priority < ?", group, model, maxPriority).
			Find(&abilities).Error
		if err != nil {
			return nil, err
		}
		if len(abilities) == 0 {
			err = DB.Where(groupCol+" = ? and model = ? and enabled = "+trueVal+" and priority = ?", group, model, maxPriority).
				Find(&abilities).Error
			if err != nil {
				return nil, err
			}
		}
	} else {
		err = DB.Where(groupCol+" = ? and model = ? and enabled = "+trueVal+" and priority = ?", group, model, maxPriority).
			Find(&abilities).Error
		if err != nil {
			return nil, err
		}
	}
	if len(abilities) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	idx := pickIndexWithSeed(seed, len(abilities))
	ability = abilities[idx]

	channel := Channel{}
	err = DB.First(&channel, "id = ?", ability.ChannelId).Error
	return &channel, err
}

func pickIndexWithSeed(seed string, size int) int {
	if size <= 0 {
		return 0
	}
	if seed == "" {
		return rand.Intn(size)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	return int(h.Sum32() % uint32(size))
}

func (channel *Channel) AddAbilities() error {
	models_ := strings.Split(channel.Models, ",")
	models_ = utils.DeDuplication(models_)
	groups_ := strings.Split(channel.Group, ",")
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		for _, group := range groups_ {
			ability := Ability{
				Group:     group,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.Status == ChannelStatusEnabled,
				Priority:  channel.Priority,
			}
			abilities = append(abilities, ability)
		}
	}
	return DB.Create(&abilities).Error
}

func (channel *Channel) DeleteAbilities() error {
	return DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
}

// UpdateAbilities updates abilities of this channel.
// Make sure the channel is completed before calling this function.
func (channel *Channel) UpdateAbilities() error {
	// A quick and dirty way to update abilities
	// First delete all abilities of this channel
	err := channel.DeleteAbilities()
	if err != nil {
		return err
	}
	// Then add new abilities
	err = channel.AddAbilities()
	if err != nil {
		return err
	}
	return nil
}

func UpdateAbilityStatus(channelId int, status bool) error {
	return DB.Model(&Ability{}).Where("channel_id = ?", channelId).Select("enabled").Update("enabled", status).Error
}

func GetGroupModels(ctx context.Context, group string) ([]string, error) {
	groupCol := "`group`"
	trueVal := "1"
	if common.UsingPostgreSQL {
		groupCol = `"group"`
		trueVal = "true"
	}
	var models []string
	err := DB.Model(&Ability{}).Distinct("model").Where(groupCol+" = ? and enabled = "+trueVal, group).Pluck("model", &models).Error
	if err != nil {
		return nil, err
	}
	sort.Strings(models)
	return models, err
}
