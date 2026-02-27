package logger

import (
	"os"
	"time"

	"seckill/pkg/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// 封装日志代码，用于全局使用
// 全局日志变量
var Log *zap.Logger
var bufferedSyncer *zapcore.BufferedWriteSyncer

// Initlogger 初始化日志
func Initlogger() {
	//1、配置encoder 格式
	encoderconfig := zap.NewProductionEncoderConfig()
	encoderconfig.TimeKey = "time"                          //修改时间key
	encoderconfig.EncodeTime = zapcore.ISO8601TimeEncoder   //时间格式为 2006-01-02T15:04:05.000Z0700
	encoderconfig.EncodeLevel = zapcore.CapitalLevelEncoder //日志级别大写输出
	//2、配置core
	//在k8s环境中，日志统一输出到stdout，由k8s收集
	level := zapcore.InfoLevel
	switch config.Get().Log.Level {
	case "debug":
		level = zapcore.DebugLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	}

	bufferedSyncer = &zapcore.BufferedWriteSyncer{
		WS:            zapcore.AddSync(os.Stdout),
		FlushInterval: 500 * time.Millisecond,
		Size:          1024 * 256,
	}

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderconfig), //使用json格式
		bufferedSyncer,
		level,
	)
	//3、构建logger
	//打印行号
	Log = zap.New(core, zap.AddCaller())
	//(可选) 替换全局logger
	zap.ReplaceGlobals(Log)
}

// 刷新日志缓冲区，在main函数退出前调用
func Sync() {
	if bufferedSyncer != nil {
		bufferedSyncer.Stop()
	}

	if err := Log.Sync(); err != nil {
		// Sync 可能会在标准输出/错误输出不支持同步时失败
		// 这通常发生在终端重定向的情况下
		// 由于这是程序退出时调用，我们只能忽略这个错误
		// 参考: https://github.com/uber-go/zap/issues/328
		_ = err
	}
}
