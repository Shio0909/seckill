package model

import "gorm.io/gorm"

type Order struct {
	gorm.Model
	UserID    uint `gorm:"not null;index:idx_user_product,unique"` // 联合唯一索引
	ProductID uint `gorm:"not null;index:idx_user_product,unique"` // 联合唯一索引

	Status   int    `gorm:"default:0"`               // 0:未支付, 1:已支付, 2:已取消
	OrderNum string `gorm:"type:varchar(32);unique"` // 订单号 (用雪花算法生成)

	// 关联关系 (可选，为了查询方便)
	Product Product `gorm:"foreignKey:ProductID"`
	User    User    `gorm:"foreignKey:UserID"`
}
