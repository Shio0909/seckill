package snowflake

import (
	"seckill/pkg/logger"

	"github.com/bwmarrin/snowflake"

	"go.uber.org/zap"
)

// 用于项目中雪花算法的实现
var node *snowflake.Node

// Init初始化雪花算法节点
// machineID 机器ID
func Init(machineID int64) {
	var err error
	node, err = snowflake.NewNode(machineID)
	if err != nil {
		logger.Log.Fatal("雪花算法节点初始化失败", zap.Error(err))
	}
	logger.Log.Info("雪花算法节点初始化成功")
}

// GenerateID生成string类型的ID
func GenerateID() string {
	id := node.Generate()
	return id.String()
}
