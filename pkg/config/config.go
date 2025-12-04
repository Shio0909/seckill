package config

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// =============================================================================
// 全局配置实例
// =============================================================================

var (
	Conf     *Config   // 全局配置对象
	once     sync.Once // 确保只初始化一次
	confLock sync.RWMutex
)

// =============================================================================
// 配置结构体定义
// =============================================================================

// Config 根配置结构
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	MySQL    MySQLConfig    `mapstructure:"mysql"`
	Redis    RedisConfig    `mapstructure:"redis"`
	RabbitMQ RabbitMQConfig `mapstructure:"rabbitmq"`
	Kafka    KafkaConfig    `mapstructure:"kafka"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	Log      LogConfig      `mapstructure:"log"`
	Consul   ConsulConfig   `mapstructure:"consul"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Name         string        `mapstructure:"name"`          // 服务名称
	Port         int           `mapstructure:"port"`          // 监听端口
	Mode         string        `mapstructure:"mode"`          // 运行模式: debug/release/test
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`  // 读超时
	WriteTimeout time.Duration `mapstructure:"write_timeout"` // 写超时
}

// MySQLConfig MySQL 数据库配置
type MySQLConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	Database        string        `mapstructure:"database"`
	Charset         string        `mapstructure:"charset"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`    // 最大空闲连接数
	MaxOpenConns    int           `mapstructure:"max_open_conns"`    // 最大打开连接数
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"` // 连接最大存活时间
	LogLevel        string        `mapstructure:"log_level"`         // SQL 日志级别: silent/error/warn/info
}

// DSN 生成 MySQL 连接字符串
func (m *MySQLConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		m.User, m.Password, m.Host, m.Port, m.Database, m.Charset)
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addr         string        `mapstructure:"addr"`
	Password     string        `mapstructure:"password"`
	DB           int           `mapstructure:"db"`
	PoolSize     int           `mapstructure:"pool_size"`      // 连接池大小
	MinIdleConns int           `mapstructure:"min_idle_conns"` // 最小空闲连接数
	DialTimeout  time.Duration `mapstructure:"dial_timeout"`   // 连接超时
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`   // 读超时
	WriteTimeout time.Duration `mapstructure:"write_timeout"`  // 写超时
}

// RabbitMQConfig RabbitMQ 配置
type RabbitMQConfig struct {
	URL       string `mapstructure:"url"`        // amqp://user:pass@host:port/vhost
	QueueName string `mapstructure:"queue_name"` // 队列名称
}

// KafkaConfig Kafka 配置（预留）
type KafkaConfig struct {
	Brokers []string `mapstructure:"brokers"`  // Broker 地址列表
	Topic   string   `mapstructure:"topic"`    // 主题名称
	GroupID string   `mapstructure:"group_id"` // 消费者组 ID
}

// JWTConfig JWT 配置
type JWTConfig struct {
	Secret     string        `mapstructure:"secret"`      // 签名密钥
	Issuer     string        `mapstructure:"issuer"`      // 签发者
	ExpireTime time.Duration `mapstructure:"expire_time"` // 过期时间
}

// LogConfig 日志配置
type LogConfig struct {
	Level      string `mapstructure:"level"`       // 日志级别: debug/info/warn/error
	Format     string `mapstructure:"format"`      // 日志格式: json/console
	OutputPath string `mapstructure:"output_path"` // 输出路径: stdout/文件路径
	MaxSize    int    `mapstructure:"max_size"`    // 单文件最大大小(MB)
	MaxBackups int    `mapstructure:"max_backups"` // 最大保留文件数
	MaxAge     int    `mapstructure:"max_age"`     // 最大保留天数
	Compress   bool   `mapstructure:"compress"`    // 是否压缩
}

// ConsulConfig Consul 配置（预留）
type ConsulConfig struct {
	Addr        string `mapstructure:"addr"`         // Consul 地址
	ServiceName string `mapstructure:"service_name"` // 注册的服务名
	ServicePort int    `mapstructure:"service_port"` // 服务端口
	HealthCheck string `mapstructure:"health_check"` // 健康检查路径
}

// =============================================================================
// 配置初始化
// =============================================================================

// InitConfig 初始化配置
// configPath: 配置文件路径，支持不带后缀（自动识别 yaml/yml/json）
// 示例: InitConfig("config/config") 或 InitConfig("./config/config.yaml")
func InitConfig(configPath string) error {
	var initErr error

	once.Do(func() {
		v := viper.New()

		// 1. 设置配置文件
		if configPath != "" {
			v.SetConfigFile(configPath)
		} else {
			// 默认配置文件位置
			v.SetConfigName("config")        // 配置文件名（不带后缀）
			v.SetConfigType("yaml")          // 配置文件类型
			v.AddConfigPath(".")             // 当前目录
			v.AddConfigPath("./config")      // config 目录
			v.AddConfigPath("../config")     // 上级 config 目录
			v.AddConfigPath("/etc/seckill/") // Linux 系统配置目录
		}

		// 2. 读取配置文件
		if err := v.ReadInConfig(); err != nil {
			initErr = fmt.Errorf("读取配置文件失败: %w", err)
			return
		}

		// 3. 环境变量覆盖
		// 示例: SECKILL_SERVER_PORT=9090 会覆盖 server.port
		v.SetEnvPrefix("SECKILL")                           // 环境变量前缀
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))  // server.port -> SERVER_PORT
		v.AutomaticEnv()                                    // 自动读取环境变量

		// 4. 解析到结构体
		Conf = &Config{}
		if err := v.Unmarshal(Conf); err != nil {
			initErr = fmt.Errorf("解析配置文件失败: %w", err)
			return
		}

		// 5. 设置默认值（如果配置文件中没有）
		setDefaults()

		// 6. 监听配置文件变化（热更新）
		v.WatchConfig()
		v.OnConfigChange(func(e fsnotify.Event) {
			fmt.Printf("配置文件变更: %s\n", e.Name)
			confLock.Lock()
			defer confLock.Unlock()
			if err := v.Unmarshal(Conf); err != nil {
				fmt.Printf("重新加载配置失败: %v\n", err)
			} else {
				fmt.Println("配置已热更新")
			}
		})

		fmt.Printf("✅ 配置加载成功: %s\n", v.ConfigFileUsed())
	})

	return initErr
}

// setDefaults 设置默认值
func setDefaults() {
	// Server 默认值
	if Conf.Server.Port == 0 {
		Conf.Server.Port = 8080
	}
	if Conf.Server.Mode == "" {
		Conf.Server.Mode = "debug"
	}
	if Conf.Server.ReadTimeout == 0 {
		Conf.Server.ReadTimeout = 10 * time.Second
	}
	if Conf.Server.WriteTimeout == 0 {
		Conf.Server.WriteTimeout = 10 * time.Second
	}

	// MySQL 默认值
	if Conf.MySQL.Charset == "" {
		Conf.MySQL.Charset = "utf8mb4"
	}
	if Conf.MySQL.MaxIdleConns == 0 {
		Conf.MySQL.MaxIdleConns = 10
	}
	if Conf.MySQL.MaxOpenConns == 0 {
		Conf.MySQL.MaxOpenConns = 100
	}
	if Conf.MySQL.ConnMaxLifetime == 0 {
		Conf.MySQL.ConnMaxLifetime = time.Hour
	}
	if Conf.MySQL.LogLevel == "" {
		Conf.MySQL.LogLevel = "info"
	}

	// Redis 默认值
	if Conf.Redis.PoolSize == 0 {
		Conf.Redis.PoolSize = 100
	}
	if Conf.Redis.MinIdleConns == 0 {
		Conf.Redis.MinIdleConns = 10
	}
	if Conf.Redis.DialTimeout == 0 {
		Conf.Redis.DialTimeout = 5 * time.Second
	}
	if Conf.Redis.ReadTimeout == 0 {
		Conf.Redis.ReadTimeout = 3 * time.Second
	}
	if Conf.Redis.WriteTimeout == 0 {
		Conf.Redis.WriteTimeout = 3 * time.Second
	}

	// RabbitMQ 默认值
	if Conf.RabbitMQ.QueueName == "" {
		Conf.RabbitMQ.QueueName = "seckill_queue"
	}

	// JWT 默认值
	if Conf.JWT.ExpireTime == 0 {
		Conf.JWT.ExpireTime = 72 * time.Hour
	}
	if Conf.JWT.Issuer == "" {
		Conf.JWT.Issuer = "seckill"
	}

	// Log 默认值
	if Conf.Log.Level == "" {
		Conf.Log.Level = "info"
	}
	if Conf.Log.Format == "" {
		Conf.Log.Format = "json"
	}
	if Conf.Log.OutputPath == "" {
		Conf.Log.OutputPath = "stdout"
	}
}

// =============================================================================
// 辅助方法
// =============================================================================

// Get 获取当前配置（读锁保护）
func Get() *Config {
	confLock.RLock()
	defer confLock.RUnlock()
	return Conf
}

// GetServerAddr 获取服务监听地址
func GetServerAddr() string {
	return fmt.Sprintf(":%d", Get().Server.Port)
}

// IsDebugMode 是否为调试模式
func IsDebugMode() bool {
	return Get().Server.Mode == "debug"
}

// IsReleaseMode 是否为生产模式
func IsReleaseMode() bool {
	return Get().Server.Mode == "release"
}
