package logger

import (
	"fmt"
	"io"
	"log"
	"os"

	"groot/internal/i18n"
)

// 日志级别
const (
	LevelDebug = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelSilent
)

// ANSI 颜色码
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
)

var (
	logger     *log.Logger
	logLevel   = LevelInfo
	logOutput  io.Writer = os.Stderr
	timeFormat = "2006-01-02 15:04:05"
	colorize   = true // 默认启用颜色
)

func init() {
	logger = log.New(logOutput, "", log.LstdFlags)
	// 检测是否支持颜色（非 Windows 且输出为终端）
	if fi, err := os.Stderr.Stat(); err == nil {
		if fi.Mode()&os.ModeCharDevice == 0 {
			colorize = false
		}
	}
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

// col 返回带颜色的标签，colorize=false 时返回纯文本
func col(color, label string) string {
	if colorize {
		return color + label + colorReset
	}
	return label
}

// Debug 输出调试日志
func Debug(format string, v ...interface{}) {
	if logLevel <= LevelDebug {
		logger.Printf(col(colorBlue, "[DEBUG] ")+format, v...)
	}
}

// Info 输出信息日志
func Info(format string, v ...interface{}) {
	if logLevel <= LevelInfo {
		logger.Printf(col(colorGreen, "[INFO] ")+format, v...)
	}
}

// Warn 输出警告日志
func Warn(format string, v ...interface{}) {
	if logLevel <= LevelWarn {
		logger.Printf(col(colorYellow, "[WARN] ")+format, v...)
	}
}

// Error 输出错误日志
func Error(format string, v ...interface{}) {
	if logLevel <= LevelError {
		logger.Printf(col(colorRed, "[ERROR] ")+format, v...)
	}
}

// Fatal 输出致命错误日志并退出
func Fatal(format string, v ...interface{}) {
	logger.Printf(col(colorRed, "[FATAL] ")+format, v...)
	os.Exit(1)
}

// FatalIfError 如果有错误则输出致命错误日志并退出
func FatalIfError(err error, format string, v ...interface{}) {
	if err != nil {
		msg := fmt.Sprintf(format, v...)
		Fatal("%s: %v", msg, err)
	}
}

// ColoredBanner 返回彩色横幅字符串（不带换行）
func ColoredBanner() string {
	banner := i18n.T("logger.banner")
	if colorize {
		return fmt.Sprintf("%s%s%s", colorGreen, banner, colorReset)
	}
	return banner
}
