package model

import "gorm.io/gorm"

type User struct {
	gorm.Model
	username string `gorm:"type:varchar(50);uniqueIndex;not null"` // 用户名
	password string `gorm:"type:varchar(100);not null"`            // 密码
	phone    string `gorm:"type:varchar(20);uniqueIndex;not null"` // 手机号
}
