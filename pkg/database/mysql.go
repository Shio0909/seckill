package database

import (
	"fmt"
	"log"
	"strings"

	"seckill/pkg/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitMySQL() {
	cfg := config.Get().MySQL

	// 使用配置生成 DSN
	dsn := cfg.DSN()

	// 根据配置设置日志级别
	var logLevel logger.LogLevel
	switch strings.ToLower(cfg.LogLevel) {
	case "silent":
		logLevel = logger.Silent
	case "error":
		logLevel = logger.Error
	case "warn":
		logLevel = logger.Warn
	default:
		logLevel = logger.Info
	}

	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})

	if err != nil {
		log.Fatalf("连接 MySQL 失败: %v", err)
	}

	sqlDB, _ := DB.DB()
	// 从配置读取连接池参数
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	fmt.Printf("✅ MySQL 连接成功 [%s:%d/%s]\n", cfg.Host, cfg.Port, cfg.Database)
}
