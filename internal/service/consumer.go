package service

import (
	"encoding/json"
	"fmt"
	"seckill/internal/model"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/rabbitmq"
	"seckill/pkg/snowflake"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

//处理消息队列的消费者
//流程：连上 RabbitMQ -> 监听队列 -> 收到消息 -> 解析json -> 开启数据库事务 -> 扣库存 -> 创建订单 -> ack确认

// startConsumer 启动消费者
func StartConsumer() {
	//1、获取channel
	ch := rabbitmq.Channel

	//2、监听队列
	msgs, err := ch.Consume(
		rabbitmq.QueueName, //队列名称
		"",                 //消费者名称
		false,              //autoAck 是否自动确认
		false,              //exclusive 是否排他
		false,              //noLocal 是否本地
		false,              //noWait 是否阻塞
		nil,                //args其他参数
	)
	if err != nil {
		logger.Log.Fatal("[Worker]消费者启动失败", zap.Error(err))
	}
	//3、开启协程处理消息
	go func() {
		logger.Log.Info("[Worker]消费者启动成功，开始监听队列")
		for d := range msgs {
			//4、解析json
			var msg rabbitmq.OrderMessage
			json.Unmarshal(d.Body, &msg)
			logger.Log.Info("收到消息", zap.Int64("uid", msg.UserID), zap.Int64("pid", msg.ProductID))
			//5、处理下单逻辑(写入mysql)
			err := createOrderInDB(msg.UserID, msg.ProductID)
			if err != nil {
				//失败处理
				logger.Log.Error("下单失败", zap.Error(err))
				//d.Reject(true) //退回队列重试
			} else {
				//处理成功 发送ack
				d.Ack(false)
			}
		}
	}()
}

// createOrderInDB 数据库事务操作 扣减mysql库存和创建订单
func createOrderInDB(uid int64, pid int64) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		//1、扣减库存
		result := tx.Model(&model.Product{}).Where("id = ? AND stock > 0", pid).
			Update("stock", gorm.Expr("stock - ?", 1))
		if result.RowsAffected == 0 {
			return fmt.Errorf("库存不足")
		}
		//2、创建订单
		order := model.Order{
			UserID:    uint(uid),
			ProductID: uint(pid),
			Status:    1, //已支付
			//订单号生成 雪花算法
			OrderNum: snowflake.GenerateID(),
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		return nil
	})
}
