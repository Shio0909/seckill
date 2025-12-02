package rabbitmq

import (
	"encoding/json"
	"seckill/pkg/logger"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// 负责连接 RabbitMQ 并创建一个名叫 seckill_queue 的队列
// 全局变量
var Conn *amqp.Connection
var Channel *amqp.Channel

// 队列名称
const QueueName = "seckill_queue"

// 初始化 RabbitMQ 连接和通道

func InitRabbitMQ() {
	//1.连接 RabbitMQ
	//格式 amqp://用户名:密码@RabbitMQ服务器地址:端口号/
	var err error
	Conn, err = amqp.Dial("amqp://guest:guest@localhost:6672/")
	if err != nil {
		logger.Log.Fatal("连接 RabbitMQ 失败", zap.Error(err))
	}

	//2.创建channel
	Channel, err = Conn.Channel()
	if err != nil {
		logger.Log.Fatal("创建 RabbitMQ 通道失败", zap.Error(err))
	}
	//3.声明队列
	//如果队列不存在则创建它，如果存在则跳过
	_, err = Channel.QueueDeclare(
		QueueName, //队列名称
		true,      //durable 是否持久化（重启后依然在）
		false,     //autoDelete 是否自动删除
		false,     //exclusive 是否排他
		false,     //noWait 是否阻塞
		nil,       //args其他参数
	)
	if err != nil {
		logger.Log.Fatal("声明队列失败", zap.Error(err))
	}
	logger.Log.Info("RabbitMQ 初始化并连接成功")
}

// ordermessage定义消息格式
type OrderMessage struct {
	UserID    int64 `json:"user_id"`
	ProductID int64 `json:"product_id"`
}

// sendseckillMessage发送消息到队列
func SendSeckillMessage(uid int64, pid int64) error {
	//1、创建消息体
	msg := OrderMessage{
		UserID:    uid,
		ProductID: pid,
	}
	//转成JSON格式
	body, _ := json.Marshal(msg)
	//2、发送消息到队列
	err := Channel.Publish(
		"",
		QueueName,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
	if err != nil {
		return err
	}
	return nil
}
