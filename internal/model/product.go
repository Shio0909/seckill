package model

import (
	"time"

	"gorm.io/gorm"
)

type Product struct {
	gorm.Model
	Name         string    `gorm:"type:varchar(100);not null"`  // 商品名称
	Price        float64   `gorm:"type:decimal(10,2);not null"` // 商品原价
	SeckillPrice float64   `gorm:"type:decimal(10,2);not null"` // 秒杀价
	Stock        int       `gorm:"not null"`                    // 库存数量
	Description  string    `gorm:"type:text"`                   // 商品描述
	ImageURL     string    `gorm:"type:varchar(255)"`           // 商品图片URL
	StartTime    time.Time `gorm:"not null"`                    // 秒杀开始时间
	EndTime      time.Time `gorm:"not null"`                    // 秒杀结束时间
}
