package logger

import (
	"fmt"
	"io"
	"log"
	"os"
)

// 日志级别
const (
	LevelDebug = iota
	LevelInfo
	LevelWarn
	LevelError
)

var (
	logger     *log.Logger
	logLevel   = LevelInfo
	logOutput  io.Writer = os.Stderr
	timeFormat = "2006-01-02 15:04:05"
)

func init() {
	logger = log.New(logOutput, "", log.LstdFlags)
}

// SetLevel 设置日志级别
func SetLevel(level int) {
	logLevel = level
}

// SetOutput 设置日志输出
func SetOutput(w io.Writer) {
	logOutput = w
	logger = log.New(logOutput, "", log.LstdFlags)
}

// Debug 输出调试日志
func Debug(format string, v ...interface{}) {
	if logLevel <= LevelDebug {
		logger.Printf("[DEBUG] "+format, v...)
	}
}

// Info 输出信息日志
func Info(format string, v ...interface{}) {
	if logLevel <= LevelInfo {
		logger.Printf("[INFO] "+format, v...)
	}
}

// Warn 输出警告日志
func Warn(format string, v ...interface{}) {
	if logLevel <= LevelWarn {
		logger.Printf("[WARN] "+format, v...)
	}
}

// Error 输出错误日志
func Error(format string, v ...interface{}) {
	if logLevel <= LevelError {
		logger.Printf("[ERROR] "+format, v...)
	}
}

// Fatal 输出致命错误日志并退出
func Fatal(format string, v ...interface{}) {
	logger.Printf("[FATAL] "+format, v...)
	os.Exit(1)
}

// FatalIfError 如果有错误则输出致命错误日志并退出
func FatalIfError(err error, format string, v ...interface{}) {
	if err != nil {
		msg := fmt.Sprintf(format, v...)
		Fatal("%s: %v", msg, err)
	}
}
