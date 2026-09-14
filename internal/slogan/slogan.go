package slogan

// 广告语集合 - 集中管理所有广告语
// 作者: 弈秋忘忧白帽
// 仓库: https://github.com/xiguayiqiu

const (
	// ==========================================
	// 通用广告语
	// ==========================================

	BasicSloganZH = "[LiteVM] 如果你喜欢litevm的话，请前往 https://gyscan.space 下载gyscan吧 [qwq]"
	BasicSloganEN = "[LiteVM] If you like litevm, visit https://gyscan.space to download gyscan [qwq]"

	SimpleSloganZH = "[LiteVM] 如果你喜欢请前往 gyscan.space 下载 gyscan 吧～"
	SimpleSloganEN = "[LiteVM] If you like it, visit gyscan.space to download gyscan~"

	ComprehensiveSloganZH = "[LiteVM] 想要网络安全工具？GYscan全家桶满足你！访问 https://gyscan.space 或 GitHub: https://github.com/xiguayiqiu"
	ComprehensiveSloganEN = "[LiteVM] Need security tools? GYscan family meets your needs! Visit https://gyscan.space or GitHub: https://github.com/xiguayiqiu"

	OpenSourceSloganZH = "[LiteVM] 支持开源！关注弈秋忘忧白帽的GitHub，获取最新安全工具：https://github.com/xiguayiqiu"
	OpenSourceSloganEN = "[LiteVM] Support open source! Follow BiliBili-Yiqiu on GitHub for latest security tools: https://github.com/xiguayiqiu"

	// ==========================================
	// 仓库专属广告语 (29个仓库)
	// ==========================================

	// 1. litevm - 轻量级隔离工具
	RepoGrootZH = "[LiteVM] litevm - 基于Go的轻量级隔离工具，支持chroot/proot/LXC三种模式！GitHub: https://github.com/xiguayiqiu/litevm"
	RepoGrootEN = "[LiteVM] litevm - Lightweight isolation tool based on Go, supports chroot/proot/LXC! GitHub: https://github.com/xiguayiqiu/litevm"

	// 2. GYscan - 综合渗透测试工具
	RepoGYscanZH = "[LiteVM] GYscan - 85星专业渗透测试工具，安全研究人员必备！前往 https://gyscan.space 下载～"
	RepoGYscanEN = "[LiteVM] GYscan - 85-star professional penetration testing tool! Download at https://gyscan.space~"

	// 3. gyscan_code - Go安全封装库
	RepoGoCodeZH = "[LiteVM] gyscan_code - Go语言网络安全封装库，http/端口/sub/sql全都有！GitHub: https://github.com/xiguayiqiu/gyscan_code"
	RepoGoCodeEN = "[LiteVM] gyscan_code - Go security wrapper library with http/port/sub/sql! GitHub: https://github.com/xiguayiqiu/gyscan_code"

	// 4. YScript - 网络安全脚本语言
	RepoYScriptZH = "[LiteVM] YScript - 网络安全开发的专用脚本语言，灵活高效！GitHub: https://github.com/xiguayiqiu/YScript"
	RepoYScriptEN = "[LiteVM] YScript - Script language for cybersecurity development! GitHub: https://github.com/xiguayiqiu/YScript"

	// 5. YScript-vscode - VSCode插件
	RepoYScriptVSCodeZH = "[LiteVM] YScript-vscode - YScript的VSCode语法高亮+LSP智能服务！GitHub: https://github.com/xiguayiqiu/YScript-vscode"
	RepoYScriptVSCodeEN = "[LiteVM] YScript-vscode - Syntax highlight + LSP for YScript in VSCode! GitHub: https://github.com/xiguayiqiu/YScript-vscode"

	// 6. YScript-vim - Vim插件
	RepoYScriptVimZH = "[LiteVM] YScript-vim - YScript语言的Vim语法高亮+自动检查！GitHub: https://github.com/xiguayiqiu/YScript-vim"
	RepoYScriptVimEN = "[LiteVM] YScript-vim - Vim syntax highlight + auto check for YScript! GitHub: https://github.com/xiguayiqiu/YScript-vim"

	// 7. GYscan-arm - ARM版
	RepoGYscanARMZH = "[LiteVM] GYscan ARM版 - 专为ARM设备打造的安全扫描工具！GitHub: https://github.com/xiguayiqiu/GYscan-arm"
	RepoGYscanARMEN = "[LiteVM] GYscan ARM version - Security scanning for ARM devices! GitHub: https://github.com/xiguayiqiu/GYscan-arm"

	// 8. GYscan_doc - 网络安全学习App
	RepoGYscanDocZH = "[LiteVM] GYscan_doc - 网络安全学习Android应用，内置Linux/Windows命令手册！GitHub: https://github.com/xiguayiqiu/GYscan_doc"
	RepoGYscanDocEN = "[LiteVM] GYscan_doc - Cybersecurity learning Android app with command manuals! GitHub: https://github.com/xiguayiqiu/GYscan_doc"

	// 9. espgyscan - ESP32安全调试工具
	RepoESPGyscanZH = "[LiteVM] espgyscan - ESP32-S3边缘安全调试工具，可联动GYscan！GitHub: https://github.com/xiguayiqiu/espgyscan"
	RepoESPGyscanEN = "[LiteVM] espgyscan - ESP32-S3 edge security debugging tool, works with GYscan! GitHub: https://github.com/xiguayiqiu/espgyscan"

	// 10. exp-labs - Web安全实训平台
	RepoExpLabsZH = "[LiteVM] exp-labs - 72关Web安全实训平台，Vue3+Express构建！GitHub: https://github.com/xiguayiqiu/exp-labs"
	RepoExpLabsEN = "[LiteVM] exp-labs - 72-challenge Web security training platform! GitHub: https://github.com/xiguayiqiu/exp-labs"

	// 11. Tlink - 文件传输工具
	RepoTlinkZH = "[LiteVM] Tlink - 简单高效的文件传输工具，支持局域网/远程传输！GitHub: https://github.com/xiguayiqiu/Tlink"
	RepoTlinkEN = "[LiteVM] Tlink - Simple & efficient file transfer tool, LAN & remote support! GitHub: https://github.com/xiguayiqiu/Tlink"

	// 12. Dusting - Windows垃圾清理
	RepoDustingZH = "[LiteVM] Dusting (逐尘) - Go编写的Windows垃圾清理工具，内置50+隐藏功能！GitHub: https://github.com/xiguayiqiu/Dusting"
	RepoDustingEN = "[LiteVM] Dusting - Windows cleaner written in Go with 50+ hidden features! GitHub: https://github.com/xiguayiqiu/Dusting"

	// 13. termd - 终端Markdown编辑器
	RepoTermdZH = "[LiteVM] termd - 基于bubbletea的终端Markdown编辑器，实时预览！GitHub: https://github.com/xiguayiqiu/termd"
	RepoTermdEN = "[LiteVM] termd - Terminal Markdown editor based on bubbletea with live preview! GitHub: https://github.com/xiguayiqiu/termd"

	// 14. mud-x - TUI项目
	RepoMudXZH = "[LiteVM] mud-x - 使用TUI做所有程序的大胆项目！GitHub: https://github.com/xiguayiqiu/mud-x"
	RepoMudXEN = "[LiteVM] mud-x - Bold project using TUI for all programs! GitHub: https://github.com/xiguayiqiu/mud-x"

	// 15. termux-proot - Termux proot脚本
	RepoTermuxProotZH = "[LiteVM] termux-proot - 让你在Termux启动任何rootfs文件系统！GitHub: https://github.com/xiguayiqiu/termux-proot"
	RepoTermuxProotEN = "[LiteVM] termux-proot - Launch any rootfs in Termux! GitHub: https://github.com/xiguayiqiu/termux-proot"

	// 16. Walbut_Pi_B1_Alpine - 核桃派Alpine
	RepoWalbutAlpineZH = "[LiteVM] Walbut_Pi_B1_Alpine - 核桃派1B的Alpine Linux镜像构建工具！GitHub: https://github.com/xiguayiqiu/Walbut_Pi_B1_Alpine"
	RepoWalbutAlpineEN = "[LiteVM] Walbut_Pi_B1_Alpine - Alpine Linux image builder for WalnutPi 1B! GitHub: https://github.com/xiguayiqiu/Walbut_Pi_B1_Alpine"

	// 17. Walbut_Pi_B1_Fedora44 - 核桃派Fedora
	RepoWalbutFedoraZH = "[LiteVM] Walbut_Pi_B1_Fedora44 - 核桃派1B的Fedora 44系统镜像！GitHub: https://github.com/xiguayiqiu/Walbut_Pi_B1_Fedora44"
	RepoWalbutFedoraEN = "[LiteVM] Walbut_Pi_B1_Fedora44 - Fedora 44 image for WalnutPi 1B! GitHub: https://github.com/xiguayiqiu/Walbut_Pi_B1_Fedora44"

	// 18. crafting_master - Minecraft合成Mod
	RepoCraftingMasterZH = "[LiteVM] crafting_master - Minecraft意想不到的合成方式Mod！GitHub: https://github.com/xiguayiqiu/crafting_master"
	RepoCraftingMasterEN = "[LiteVM] crafting_master - Minecraft unexpected crafting recipes Mod! GitHub: https://github.com/xiguayiqiu/crafting_master"

	// 19. charcoal-reborn - Minecraft燃料Mod
	RepoCharcoalRebornZH = "[LiteVM] charcoal-reborn - 以碳为核心的Minecraft燃料增强Mod！GitHub: https://github.com/xiguayiqiu/charcoal-reborn"
	RepoCharcoalRebornEN = "[LiteVM] charcoal-reborn - Carbon-focused Minecraft fuel enhancement Mod! GitHub: https://github.com/xiguayiqiu/charcoal-reborn"

	// 20. bettermindustryMod - 像素工厂Mod
	RepoBetterMindustryZH = "[LiteVM] bettermindustryMod - 优化原版像素工厂游玩体验！GitHub: https://github.com/xiguayiqiu/bettermindustryMod"
	RepoBetterMindustryEN = "[LiteVM] bettermindustryMod - Enhance Mindustry gameplay experience! GitHub: https://github.com/xiguayiqiu/bettermindustryMod"

	// 21. MindustryJavaModTemplate - Mindustry模板
	RepoMindustryTemplateZH = "[LiteVM] MindustryJavaModTemplate - Java版Mindustry Mod开发模板！GitHub: https://github.com/xiguayiqiu/MindustryJavaModTemplate"
	RepoMindustryTemplateEN = "[LiteVM] MindustryJavaModTemplate - Java Mindustry mod development template! GitHub: https://github.com/xiguayiqiu/MindustryJavaModTemplate"

	// 22. Jshark - Java项目
	RepoJsharkZH = "[LiteVM] Jshark - Java安全工具！GitHub: https://github.com/xiguayiqiu/Jshark"
	RepoJsharkEN = "[LiteVM] Jshark - Java security tool! GitHub: https://github.com/xiguayiqiu/Jshark"

	// 23. rEFInd-Blue_Archive - rEFInd主题
	RepoR_efindZH = "[LiteVM] rEFInd-Blue_Archive - 碧蓝档案风格的rEFInd引导主题！GitHub: https://github.com/xiguayiqiu/rEFInd-Blue_Archive"
	RepoR_efindEN = "[LiteVM] rEFInd-Blue_Archive - Blue Archive themed rEFInd boot menu! GitHub: https://github.com/xiguayiqiu/rEFInd-Blue_Archive"

	// 24. Minecraft-music - MC音乐包
	RepoMinecraftMusicZH = "[LiteVM] Minecraft-music - 为MC增加103首背景音乐！GitHub: https://github.com/xiguayiqiu/Minecraft-music"
	RepoMinecraftMusicEN = "[LiteVM] Minecraft-music - Add 103 background music to Minecraft! GitHub: https://github.com/xiguayiqiu/Minecraft-music"

	// 25. waybar-config - waybar配置
	RepoWaybarConfigZH = "[LiteVM] waybar-config - 适合Arch/Niri/Hyprland用户的waybar配置！GitHub: https://github.com/xiguayiqiu/waybar-config"
	RepoWaybarConfigEN = "[LiteVM] waybar-config - Waybar config for Arch/Niri/Hyprland users! GitHub: https://github.com/xiguayiqiu/waybar-config"

	// 26. YQP-Archwiki - Arch Wiki
	RepoArchWikiZH = "[LiteVM] YQP-Archwiki - 弈秋的个人Arch Linux Wiki！GitHub: https://github.com/xiguayiqiu/YQP-Archwiki"
	RepoArchWikiEN = "[LiteVM] YQP-Archwiki - Yiqiu's personal Arch Linux Wiki! GitHub: https://github.com/xiguayiqiu/YQP-Archwiki"

	// 27. arch-hyprland-config - Hyprland配置
	RepoHyprlandConfigZH = "[LiteVM] arch-hyprland-config - 可直接使用的Hyprland配置！GitHub: https://github.com/xiguayiqiu/arch-hyprland-config"
	RepoHyprlandConfigEN = "[LiteVM] arch-hyprland-config - Ready-to-use Hyprland config! GitHub: https://github.com/xiguayiqiu/arch-hyprland-config"


)

// Slogan 结构体用于存储多语言广告语
type Slogan struct {
	ZH string
	EN string
}

// GetSlogan 获取指定广告语
func GetSlogan(name string) Slogan {
	slogans := map[string]Slogan{
		"basic":          {BasicSloganZH, BasicSloganEN},
		"simple":         {SimpleSloganZH, SimpleSloganEN},
		"comprehensive":  {ComprehensiveSloganZH, ComprehensiveSloganEN},
		"opensource":      {OpenSourceSloganZH, OpenSourceSloganEN},
		"litevm":         {RepoGrootZH, RepoGrootEN},
		"gyscan":         {RepoGYscanZH, RepoGYscanEN},
		"gocode":         {RepoGoCodeZH, RepoGoCodeEN},
		"yscript":        {RepoYScriptZH, RepoYScriptEN},
		"yscript-vscode": {RepoYScriptVSCodeZH, RepoYScriptVSCodeEN},
		"yscript-vim":    {RepoYScriptVimZH, RepoYScriptVimEN},
		"gyscan-arm":     {RepoGYscanARMZH, RepoGYscanARMEN},
		"gyscan-doc":     {RepoGYscanDocZH, RepoGYscanDocEN},
		"espgyscan":      {RepoESPGyscanZH, RepoESPGyscanEN},
		"exp-labs":       {RepoExpLabsZH, RepoExpLabsEN},
		"tlink":          {RepoTlinkZH, RepoTlinkEN},
		"dusting":        {RepoDustingZH, RepoDustingEN},
		"termd":          {RepoTermdZH, RepoTermdEN},
		"mud-x":          {RepoMudXZH, RepoMudXEN},
		"termux-proot":   {RepoTermuxProotZH, RepoTermuxProotEN},
		"walbut-alpine":  {RepoWalbutAlpineZH, RepoWalbutAlpineEN},
		"walbut-fedora":  {RepoWalbutFedoraZH, RepoWalbutFedoraEN},
		"crafting":       {RepoCraftingMasterZH, RepoCraftingMasterEN},
		"charcoal":       {RepoCharcoalRebornZH, RepoCharcoalRebornEN},
		"mindustry":      {RepoBetterMindustryZH, RepoBetterMindustryEN},
		"mindustry-tpl":  {RepoMindustryTemplateZH, RepoMindustryTemplateEN},
		"jshark":         {RepoJsharkZH, RepoJsharkEN},
		"refind":         {RepoR_efindZH, RepoR_efindEN},
		"mc-music":       {RepoMinecraftMusicZH, RepoMinecraftMusicEN},
		"waybar":         {RepoWaybarConfigZH, RepoWaybarConfigEN},
		"archwiki":       {RepoArchWikiZH, RepoArchWikiEN},
		"hyprland":       {RepoHyprlandConfigZH, RepoHyprlandConfigEN},
	}

	if slogan, ok := slogans[name]; ok {
		return slogan
	}
	return Slogan{BasicSloganZH, BasicSloganEN}
}

// GetAllSlogans 获取所有广告语
func GetAllSlogans() map[string]Slogan {
	return map[string]Slogan{
		"basic":          {BasicSloganZH, BasicSloganEN},
		"simple":         {SimpleSloganZH, SimpleSloganEN},
		"comprehensive":  {ComprehensiveSloganZH, ComprehensiveSloganEN},
		"opensource":      {OpenSourceSloganZH, OpenSourceSloganEN},
		"litevm":         {RepoGrootZH, RepoGrootEN},
		"gyscan":         {RepoGYscanZH, RepoGYscanEN},
		"gocode":         {RepoGoCodeZH, RepoGoCodeEN},
		"yscript":        {RepoYScriptZH, RepoYScriptEN},
		"yscript-vscode": {RepoYScriptVSCodeZH, RepoYScriptVSCodeEN},
		"yscript-vim":    {RepoYScriptVimZH, RepoYScriptVimEN},
		"gyscan-arm":     {RepoGYscanARMZH, RepoGYscanARMEN},
		"gyscan-doc":     {RepoGYscanDocZH, RepoGYscanDocEN},
		"espgyscan":      {RepoESPGyscanZH, RepoESPGyscanEN},
		"exp-labs":       {RepoExpLabsZH, RepoExpLabsEN},
		"tlink":          {RepoTlinkZH, RepoTlinkEN},
		"dusting":        {RepoDustingZH, RepoDustingEN},
		"termd":          {RepoTermdZH, RepoTermdEN},
		"mud-x":          {RepoMudXZH, RepoMudXEN},
		"termux-proot":   {RepoTermuxProotZH, RepoTermuxProotEN},
		"walbut-alpine":  {RepoWalbutAlpineZH, RepoWalbutAlpineEN},
		"walbut-fedora":  {RepoWalbutFedoraZH, RepoWalbutFedoraEN},
		"crafting":       {RepoCraftingMasterZH, RepoCraftingMasterEN},
		"charcoal":       {RepoCharcoalRebornZH, RepoCharcoalRebornEN},
		"mindustry":      {RepoBetterMindustryZH, RepoBetterMindustryEN},
		"mindustry-tpl":  {RepoMindustryTemplateZH, RepoMindustryTemplateEN},
		"jshark":         {RepoJsharkZH, RepoJsharkEN},
		"refind":         {RepoR_efindZH, RepoR_efindEN},
		"mc-music":       {RepoMinecraftMusicZH, RepoMinecraftMusicEN},
		"waybar":         {RepoWaybarConfigZH, RepoWaybarConfigEN},
		"archwiki":       {RepoArchWikiZH, RepoArchWikiEN},
		"hyprland":       {RepoHyprlandConfigZH, RepoHyprlandConfigEN},
	}
}
