package config

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// Config 主配置结构
type Config struct {
	// 路径配置
	Path string `envconfig:"PATH" default:"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"`

	// 用户配置
	DefaultUser  string `envconfig:"USER" default:"root"`
	DefaultShell string `envconfig:"SHELL" default:"/bin/sh"`

	// 网络配置
	Hostname string `envconfig:"HOSTNAME" default:"groot"`

	// 调试模式
	Debug bool `envconfig:"DEBUG" default:"false"`
}

var globalConfig *Config

// Load 加载配置
func Load() (*Config, error) {
	// 尝试从 .env 文件加载
	_ = godotenv.Load()

	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}

	globalConfig = &cfg
	return &cfg, nil
}

// Get 获取全局配置
func Get() *Config {
	if globalConfig == nil {
		cfg, _ := Load()
		return cfg
	}
	return globalConfig
}

// GetPreserveEnvVars 获取需要保留的环境变量列表
func GetPreserveEnvVars() []string {
	return []string{
		"LANG", "LC_ALL", "LC_CTYPE", "LC_NUMERIC",
		"LC_TIME", "LC_COLLATE", "LC_MONETARY", "LC_MESSAGES",
		"LC_PAPER", "LC_NAME", "LC_ADDRESS", "LC_TELEPHONE",
		"LC_MEASUREMENT", "LC_IDENTIFICATION",
		"DISPLAY", "WAYLAND_DISPLAY",
		"XDG_SESSION_TYPE", "XDG_SESSION_CLASS",
		"XDG_SESSION_ID", "XDG_RUNTIME_DIR",
		"DBUS_SESSION_BUS_ADDRESS",
		"SSH_AUTH_SOCK", "SSH_AGENT_PID",
		"COLORTERM", "TERM",
	}
}

// CleanEnv 清理并设置环境变量
func CleanEnv() {
	os.Clearenv()
}

// SetEnv 设置环境变量
func SetEnv(envMap map[string]string) {
	for key, value := range envMap {
		os.Setenv(key, value)
	}
}

// MergeEnvMaps 合并多个环境变量 map，后面的会覆盖前面的
func MergeEnvMaps(maps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range maps {
		for key, value := range m {
			result[key] = value
		}
	}
	return result
}

// EnvMapToSlice 将环境变量 map 转换为切片
func EnvMapToSlice(envMap map[string]string) []string {
	result := make([]string, 0, len(envMap))
	for key, value := range envMap {
		result = append(result, key+"="+value)
	}
	return result
}

// EnvSliceToMap 将环境变量切片转换为 map
func EnvSliceToMap(envSlice []string) map[string]string {
	result := make(map[string]string)
	for _, e := range envSlice {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}
