package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

// 全局 logger
var log zerolog.Logger

// 日志文件句柄（用于关闭）
var logFile *os.File

// 日志级别常量
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

func init() {
	// 默认初始化为彩色控制台输出（开发模式）
	Init(LevelDebug, true)
}

// Init 初始化全局 logger。
//   - level: debug, info, warn, error
//   - pretty: true 为彩色控制台输出，false 为 JSON 输出
//
// 用法：
//
//	// 开发模式（彩色控制台，默认）
//	logger.Init(logger.LevelInfo, true)
//	// 生产模式（JSON 格式）
//	logger.Init(logger.LevelInfo, false)
func Init(level string, pretty bool) {
	initWithWriter(level, pretty, nil)
}

// InitWithFile 初始化 logger 并同时输出到文件
// logDir: 日志目录路径，如 "/logs/jsonl_packer"
// 返回日志文件路径
func InitWithFile(level string, pretty bool, logDir string) (string, error) {
	// 创建日志目录
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 生成日志文件名（带时间戳）
	logFileName := fmt.Sprintf("%s.log", time.Now().Format("20060102_150405"))
	logPath := filepath.Join(logDir, logFileName)

	// 打开日志文件
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return "", fmt.Errorf("打开日志文件失败: %w", err)
	}

	// 保存文件句柄
	logFile = file

	// 初始化 logger（同时输出到控制台和文件）
	initWithWriter(level, pretty, file)

	return logPath, nil
}

// initWithWriter 内部初始化函数
func initWithWriter(level string, pretty bool, fileWriter io.Writer) {
	// 设置日志级别
	var zeroLevel zerolog.Level
	switch level {
	case LevelDebug:
		zeroLevel = zerolog.DebugLevel
	case LevelInfo:
		zeroLevel = zerolog.InfoLevel
	case LevelWarn:
		zeroLevel = zerolog.WarnLevel
	case LevelError:
		zeroLevel = zerolog.ErrorLevel
	default:
		zeroLevel = zerolog.DebugLevel
	}

	zerolog.SetGlobalLevel(zeroLevel)

	if pretty {
		// 彩色控制台输出（开发模式）
		consoleWriter := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "2006/01/02 15:04:05",
			NoColor:    false,
		}

		if fileWriter != nil {
			// 同时输出到控制台和文件
			// 文件使用无颜色的格式
			fileConsoleWriter := zerolog.ConsoleWriter{
				Out:        fileWriter,
				TimeFormat: "2006/01/02 15:04:05",
				NoColor:    true, // 文件不需要颜色
			}
			multiWriter := io.MultiWriter(consoleWriter, fileConsoleWriter)
			log = zerolog.New(multiWriter).With().Timestamp().Logger()
		} else {
			log = zerolog.New(consoleWriter).With().Timestamp().Logger()
		}
	} else {
		// JSON 输出（生产模式）
		zerolog.TimeFieldFormat = "2006/01/02 15:04:05"

		if fileWriter != nil {
			// 同时输出到控制台和文件
			multiWriter := io.MultiWriter(os.Stdout, fileWriter)
			log = zerolog.New(multiWriter).With().Timestamp().Logger()
		} else {
			log = zerolog.New(os.Stdout).With().Timestamp().Logger()
		}
	}
}

// Close 关闭日志文件
func Close() {
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}

// ==================== 简洁风格（类似 Python loguru）====================

// Debugf 调试日志
func Debugf(format string, v ...interface{}) {
	log.Debug().Msgf(format, v...)
}

// Infof 信息日志
func Infof(format string, v ...interface{}) {
	log.Info().Msgf(format, v...)
}

// Warnf 警告日志
func Warnf(format string, v ...interface{}) {
	log.Warn().Msgf(format, v...)
}

// Errorf 错误日志
func Errorf(format string, v ...interface{}) {
	log.Error().Msgf(format, v...)
}

// ErrorfE 错误日志并返回 error（一行代码同时记录日志和返回错误）
func ErrorfE(format string, v ...interface{}) error {
	log.Error().Msgf(format, v...)
	return fmt.Errorf(format, v...)
}

// Fatalf 致命错误日志（会调用 os.Exit(1)）
func Fatalf(format string, v ...interface{}) {
	log.Fatal().Msgf(format, v...)
}

// ==================== 链式风格（需要结构化字段时使用）====================

// Debug 调试日志（链式）
func Debug() *zerolog.Event {
	return log.Debug()
}

// Info 信息日志（链式）
func Info() *zerolog.Event {
	return log.Info()
}

// Warn 警告日志（链式）
func Warn() *zerolog.Event {
	return log.Warn()
}

// Error 错误日志（链式）
func Error() *zerolog.Event {
	return log.Error()
}

// Fatal 致命错误日志（链式，会调用 os.Exit(1)）
func Fatal() *zerolog.Event {
	return log.Fatal()
}

// ==================== 工具函数 ====================

// SetLevel 动态设置日志级别
func SetLevel(level string) {
	var zeroLevel zerolog.Level
	switch level {
	case LevelDebug:
		zeroLevel = zerolog.DebugLevel
	case LevelInfo:
		zeroLevel = zerolog.InfoLevel
	case LevelWarn:
		zeroLevel = zerolog.WarnLevel
	case LevelError:
		zeroLevel = zerolog.ErrorLevel
	default:
		zeroLevel = zerolog.DebugLevel
	}
	zerolog.SetGlobalLevel(zeroLevel)
}
