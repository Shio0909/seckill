package model

import (
	"time"

	"gorm.io/gorm"
)

type Product struct {
	gorm.Model
	Name        string    `gorm:"type:varchar(200);not null"`  // 活动名称
	CategoryID  int       `gorm:"default:1"`                   // 类别: 1演唱会 2体育 3展览 4话剧 5音乐节
	CityID      int       `gorm:"default:1"`                   // 城市ID
	City        string    `gorm:"type:varchar(50)"`            // 城市名称
	Venue       string    `gorm:"type:varchar(200)"`           // 场馆
	Price       float64   `gorm:"type:decimal(10,2);not null"` // 最低票价
	HighPrice   float64   `gorm:"type:decimal(10,2)"`          // 最高票价
	Stock       int       `gorm:"not null"`                    // 总票数
	Description string    `gorm:"type:text"`                   // 活动描述
	ImageURL    string    `gorm:"type:varchar(255)"`           // 海报图片
	Artist      string    `gorm:"type:varchar(100)"`           // 艺人/球队
	Tags        string    `gorm:"type:varchar(500)"`           // 标签，逗号分隔
	HotScore    float64   `gorm:"default:0"`                   // 热度分数
	StartTime   time.Time `gorm:"not null"`                    // 开票时间（抢票开始）
	EndTime     time.Time `gorm:"not null"`                    // 开票结束时间
	EventTime   time.Time `gorm:"not null"`                    // 活动开始时间
}

// CategoryName 获取类别名称
func (p *Product) CategoryName() string {
	categories := map[int]string{
		1: "演唱会",
		2: "体育赛事",
		3: "展览",
		4: "话剧",
		5: "音乐节",
	}
	if name, ok := categories[p.CategoryID]; ok {
		return name
	}
	return "其他"
}
