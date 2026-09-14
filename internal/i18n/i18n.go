package i18n

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Lang 语言类型
type Lang string

const (
	LangEN Lang = "en"
	LangZH Lang = "zh"
)

// TermuxHome Termux 固定 home 路径
const TermuxHome = "/data/data/com.termux/files/home"

// currentLang 当前语言
var currentLang Lang = LangEN

// configPath 语言配置文件路径 (~/.config/litevm/lang)
var configPath string

// Init 初始化国际化，检测语言设置
func Init(rootfsPath ...string) {
	configPath = getLangConfigPath()

	// 1. 尝试从配置文件读取
	if lang := loadLangConfig(); lang != "" {
		currentLang = lang
		return
	}

	// 2. 尝试从 rootfs 的 locale.conf 读取
	if len(rootfsPath) > 0 && rootfsPath[0] != "" {
		if lang := detectFromLocaleConf(rootfsPath[0]); lang != "" {
			currentLang = lang
			return
		}
	}

	// 3. Termux 环境：检查 ~/.termux/locale.conf
	if isTermux() {
		termuxLocalePath := GetTermuxLocalePath()
		if lang := detectFromTermuxLocaleConf(termuxLocalePath); lang != "" {
			currentLang = lang
			return
		}
		// 无 locale.conf，默认英文
		currentLang = LangEN
		return
	}

	// 4. 尝试从 LANG 环境变量读取
	if lang := detectFromEnv(); lang != "" {
		currentLang = lang
		return
	}

	// 5. 默认英文
	currentLang = LangEN
}

// T 返回当前语言的翻译字符串
func T(key string) string {
	if msgs, ok := messages[key]; ok {
		if msg, ok := msgs[currentLang]; ok {
			return msg
		}
		// 回退到英文
		if msg, ok := msgs[LangEN]; ok {
			return msg
		}
	}
	return key
}

// Tf 返回格式化的翻译字符串
func Tf(key string, args ...interface{}) string {
	return fmt.Sprintf(T(key), args...)
}

// GetLang 获取当前语言
func GetLang() Lang {
	return currentLang
}

// SetLang 设置语言并保存配置
func SetLang(lang Lang) {
	currentLang = lang
	saveLangConfig(lang)
}

// getLangConfigPath 获取语言配置文件路径
func getLangConfigPath() string {
	home := getHomeDir()
	return filepath.Join(home, ".config", "litevm", "lang")
}

// getHomeDir 获取 home 目录，Termux 使用固定路径
func getHomeDir() string {
	if isTermux() {
		return TermuxHome
	}
	home := os.Getenv("HOME")
	if home == "" {
		home = "/"
	}
	return home
}

// loadLangConfig 从配置文件读取语言设置
func loadLangConfig() Lang {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}
	lang := strings.TrimSpace(string(data))
	switch lang {
	case "zh", "zh_CN", "zh-CN":
		return LangZH
	case "en", "en_US", "en-US":
		return LangEN
	}
	return ""
}

// saveLangConfig 保存语言设置到配置文件
func saveLangConfig(lang Lang) {
	dir := filepath.Dir(configPath)
	os.MkdirAll(dir, 0755)
	os.WriteFile(configPath, []byte(string(lang)), 0644)
}

// detectFromLocaleConf 从 rootfs 的 /etc/locale.conf 检测语言
func detectFromLocaleConf(rootfsPath string) Lang {
	localePath := filepath.Join(rootfsPath, "etc", "locale.conf")
	data, err := os.ReadFile(localePath)
	if err != nil {
		return ""
	}
	content := strings.ToLower(string(data))
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "lang=") {
			value := strings.Trim(strings.TrimPrefix(line, "lang="), "\"'")
			if strings.HasPrefix(value, "zh") {
				return LangZH
			}
			if strings.HasPrefix(value, "en") {
				return LangEN
			}
		}
	}
	return ""
}

// detectFromEnv 从 LANG 环境变量检测语言
func detectFromEnv() Lang {
	lang := os.Getenv("LANG")
	if lang == "" {
		lang = os.Getenv("LC_ALL")
	}
	if lang == "" {
		lang = os.Getenv("LANGUAGE")
	}
	lang = strings.ToLower(lang)
	if strings.HasPrefix(lang, "zh") {
		return LangZH
	}
	if strings.HasPrefix(lang, "en") {
		return LangEN
	}
	return ""
}

// GetTermuxLocalePath 获取 Termux locale.conf 路径
func GetTermuxLocalePath() string {
	return filepath.Join(TermuxHome, ".termux", "locale.conf")
}

// detectFromTermuxLocaleConf 从 Termux 的 ~/.termux/locale.conf 检测语言
func detectFromTermuxLocaleConf(path string) Lang {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	content := strings.ToLower(string(data))
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "lang=") {
			value := strings.Trim(strings.TrimPrefix(line, "lang="), "\"'")
			if strings.HasPrefix(value, "zh") {
				return LangZH
			}
			if strings.HasPrefix(value, "en") {
				return LangEN
			}
		}
	}
	return ""
}

// AskLanguageAndSave 交互式询问语言并保存到 Termux locale.conf
// 导出函数，供 --termux-lang 参数调用
func AskLanguageAndSave(termuxLocalePath string) {
	fmt.Println()
	fmt.Println("\033[36m[LiteVM]\033[0m \033[33mSelect Language / 选择语言:\033[0m")
	fmt.Println()
	fmt.Println("  1. English")
	fmt.Println("  2. 中文")
	fmt.Println()
	fmt.Print("\033[36m请选择 / Enter choice [1-2]: \033[0m")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	var lang Lang
	var localeValue string
	switch input {
	case "2", "zh", "cn", "中文":
		lang = LangZH
		localeValue = "zh_CN.UTF-8"
	default:
		lang = LangEN
		localeValue = "en_US.UTF-8"
	}

	// 保存语言设置
	currentLang = lang

	// 生成 ~/.termux/locale.conf
	dir := filepath.Dir(termuxLocalePath)
	os.MkdirAll(dir, 0755)
	localeConf := fmt.Sprintf("LANG=%s\n", localeValue)
	os.WriteFile(termuxLocalePath, []byte(localeConf), 0644)

	fmt.Printf("\033[32m已保存语言设置到 %s\033[0m\n", termuxLocalePath)
	fmt.Println()
}

// ShowTermuxLangTUI 显示 Termux 语言选择 TUI（供 --termux-lang 调用）
func ShowTermuxLangTUI() {
	termuxLocalePath := GetTermuxLocalePath()

	// 显示当前配置
	fmt.Println("\033[36m[LiteVM] Termux 语言配置\033[0m")
	fmt.Println()

	if data, err := os.ReadFile(termuxLocalePath); err == nil {
		fmt.Printf("\033[33m当前配置 (%s):\033[0m\n", termuxLocalePath)
		fmt.Printf("  %s\n", strings.TrimSpace(string(data)))
	} else {
		fmt.Printf("\033[33m配置文件不存在: %s\033[0m\n", termuxLocalePath)
	}
	fmt.Println()

	AskLanguageAndSave(termuxLocalePath)

	// 提示重启
	fmt.Println("\033[33m提示: 请重启 Termux 使语言设置生效\033[0m")
	fmt.Println()
}

// isTermux 内联检测 Termux 环境，避免循环依赖
func isTermux() bool {
	if _, ok := os.LookupEnv("TERMUX_VERSION"); ok {
		return true
	}
	prefix := os.Getenv("PREFIX")
	if prefix != "" && strings.Contains(prefix, "com.termux") {
		return true
	}
	return false
}
