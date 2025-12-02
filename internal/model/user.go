package model

import "gorm.io/gorm"

type User struct {
	gorm.Model
	Username string `gorm:"type:varchar(50);uniqueIndex;not null"` // 用户名
	Password string `gorm:"type:varchar(100);not null"`            // 密码
	Phone    string `gorm:"type:varchar(20);uniqueIndex;not null"` // 手机号
	Email    string `gorm:"type:varchar(100);comment:邮箱"`
	Avatar   string `gorm:"type:varchar(255);comment:头像URL"`
	Status   int    `gorm:"default:1;comment:状态 1:正常 2:禁用"`
	IsAdmin  bool   `gorm:"default:false;comment:是否管理员"`
}
