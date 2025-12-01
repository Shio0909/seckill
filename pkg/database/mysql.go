package database

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitMySQL() {
	// 本地开发连 K8s 用 localhost (因为做了端口转发)
	// 部署上线时要改成 mysql-service (通过配置文件控制，这里先写死调试)
	dsn := "root:root123456@tcp(127.0.0.1:3307)/seckill?charset=utf8mb4&parseTime=True&loc=Local"

	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info), // 打印 SQL 日志，方便调试
	})

	if err != nil {
		log.Fatalf("连接 MySQL 失败: %v", err)
	}

	sqlDB, _ := DB.DB()
	// 设置连接池
	sqlDB.SetMaxIdleConns(10)  // 空闲连接数
	sqlDB.SetMaxOpenConns(100) // 最大连接数
	sqlDB.SetConnMaxLifetime(time.Hour)

	fmt.Println("✅ MySQL 连接成功")
}
