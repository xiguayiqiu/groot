package slogan

import (
	"fmt"
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// GetRandomSlogan 获取随机广告语
func GetRandomSlogan() Slogan {
	slogans := GetAllSlogans()
	keys := make([]string, 0, len(slogans))
	for k := range slogans {
		keys = append(keys, k)
	}
	randomKey := keys[rand.Intn(len(keys))]
	return slogans[randomKey]
}

// GetBanner 获取带颜色的广告横幅
func GetBanner(sloganName string) string {
	slogan := GetSlogan(sloganName)
	return fmt.Sprintf("\\033[36m%s\\033[0m", slogan.ZH)
}

// GetRandomBanner 获取随机广告横幅
func GetRandomBanner() string {
	slogan := GetRandomSlogan()
	return fmt.Sprintf("\\033[36m%s\\033[0m", slogan.ZH)
}

// GetBannerCmd 获取printf命令（用于shell启动前执行）
func GetBannerCmd() string {
	slogan := GetRandomSlogan()
	return fmt.Sprintf("printf '\\033[36m%s\\033[0m\\n'", slogan.ZH)
}

// PrintSlogan 打印广告语
func PrintSlogan(sloganName string) {
	slogan := GetSlogan(sloganName)
	fmt.Printf("\033[36m%s\033[0m\n", slogan.ZH)
}
