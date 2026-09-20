package mod

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"log"

	"dst-admin-go/internal/pkg/utils/fileUtils"
)

// workshopIdPattern 匹配 modoverrides.lua 中的 ["workshop-<id>"] 启用项
var workshopIdPattern = regexp.MustCompile(`workshop-(\d+)`)

// enabledWorkshopIds 从 modoverrides.lua 原文提取启用的创意工坊模组 ID。
// modoverrides.lua 是世界加载模组的唯一真相（spec C1），播种以它为清单来源。
func enabledWorkshopIds(modoverridesContent string) []string {
	matches := workshopIdPattern.FindAllStringSubmatch(modoverridesContent, -1)
	seen := make(map[string]bool, len(matches))
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		if !seen[m[1]] {
			seen[m[1]] = true
			ids = append(ids, m[1])
		}
	}
	return ids
}

// SeedUgcCache 启动世界前从面板备份库播种游戏 UGC 缓存。
//
// 背景：box64/steamclient 下游戏启动时的创意工坊自下载慢且不稳
// （ODPF failed entirely / DownloadServerMods timed out），启用清单里的模组
// 缺装时游戏只会带着已就位的部分开服，且现场下载超时后即使迟到也不会生效。
//
// 规则（对齐 dst-mod-management spec）：
//   - 以该世界 modoverrides.lua 的启用集合为清单（C1）
//   - 游戏缓存按 shard 各持一份，逐世界独立播种（C3）
//   - 目录已含 modinfo.lua 视为在位，仅补 ACF 登记；否则从备份库拷贝
//   - 备份库缺目录时跳过（保留游戏自下载现状），不报错不阻塞启动
func (s *ModService) SeedUgcCache(clusterName, levelName string) {
	logger := log.New(os.Stderr, "", log.LstdFlags)

	modoverridesPath := filepath.Join(s.pathResolver.LevelPath(clusterName, levelName), "modoverrides.lua")
	content, err := os.ReadFile(modoverridesPath)
	if err != nil {
		// 无 modoverrides（未配置模组的世界）属正常情况，静默跳过
		return
	}
	ids := enabledWorkshopIds(string(content))
	if len(ids) == 0 {
		return
	}

	config, err := s.dstConfig.GetDstConfig(clusterName)
	if err != nil {
		logger.Println("UGC 播种跳过：读取面板配置失败", clusterName, err)
		return
	}
	acfPath := s.pathResolver.GetUgcAcfPath(clusterName, levelName)

	for _, id := range ids {
		target := s.pathResolver.GetUgcWorkshopModPath(clusterName, levelName, id)
		source := workshopModPath(config.Mod_download_path, id)

		if !fileUtils.Exists(filepath.Join(target, "modinfo.lua")) {
			// 备份库可能仍是 zip 未解压形态，先走既有解压兜底
			if !fileUtils.Exists(filepath.Join(source, "modinfo.lua")) {
				s.extractModZip(source)
			}
			if !fileUtils.Exists(filepath.Join(source, "modinfo.lua")) {
				logger.Println("UGC 播种跳过（备份库缺该模组，留给游戏自下载）", clusterName, levelName, id)
				continue
			}
			// fileUtils.Copy 目录语义：内容落到 outFileDir/<src目录名>，即 target 本身
			if err := fileUtils.Copy(source, filepath.Dir(target)); err != nil {
				logger.Println("UGC 播种拷贝失败", clusterName, levelName, id, err)
				continue
			}
			logger.Println("UGC 播种完成", clusterName, levelName, id)
		}

		// 目录在位后确保 ACF 两段登记存在：游戏启动以 ACF 判定“already have”，
		// 只拷目录不登记时游戏仍会联网下载该模组（实测 4 个无 WorkshopID.txt 但
		// 有登记的模组启动即被识别，登记才是识别依据）。
		if err := ensureAcfEntry(acfPath, id); err != nil {
			logger.Println("UGC 播种补写 ACF 登记失败", clusterName, levelName, id, err)
		}
	}
}

// ensureAcfEntry 确保 appworkshop_322330.acf 中该模组两段登记齐全，逐段独立补写：
// 只有 WorkshopItemDetails 而缺 WorkshopItemsInstalled 时游戏视为未安装，
// 仍会联网重下且不加载（实测 661253977/1898181913 details-only 即此症状）。
// 文件不存在时创建含两段骨架的最小 ACF。manifest 未知记 -1：游戏如据此判定
// 需更新会自行重下，此时目录已在位，更新失败也不影响本次加载。
func ensureAcfEntry(acfPath, workshopId string) error {
	now := strconv.FormatInt(time.Now().Unix(), 10)
	size := strconv.FormatInt(dirSize(filepath.Dir(acfPath), workshopId), 10)
	installedEntry := acfEntryBlock(workshopId, [][2]string{
		{"size", size},
		{"timeupdated", now},
		{"manifest", "-1"},
	})
	detailsEntry := acfEntryBlock(workshopId, [][2]string{
		{"manifest", "-1"},
		{"timeupdated", now},
		{"timetouched", now},
		{"latest_timeupdated", now},
		{"latest_manifest", "-1"},
	})

	content, err := os.ReadFile(acfPath)
	if err != nil {
		skeleton := "\"AppWorkshop\"\n{\n\t\"appid\"\t\t\"322330\"\n" +
			"\t\"WorkshopItemsInstalled\"\n\t{\n" + installedEntry + "\t}\n" +
			"\t\"WorkshopItemDetails\"\n\t{\n" + detailsEntry + "\t}\n}\n"
		fileUtils.CreateDirIfNotExists(filepath.Dir(acfPath))
		return os.WriteFile(acfPath, []byte(skeleton), 0644)
	}

	existing := string(content)
	updated := existing
	if !acfSectionContains(updated, "WorkshopItemsInstalled", workshopId) {
		updated = insertAcfSectionEntry(updated, "WorkshopItemsInstalled", installedEntry)
	}
	if !acfSectionContains(updated, "WorkshopItemDetails", workshopId) {
		updated = insertAcfSectionEntry(updated, "WorkshopItemDetails", detailsEntry)
	}
	if updated == existing {
		return nil
	}
	return os.WriteFile(acfPath, []byte(updated), 0644)
}

// acfSectionContains 判断 ID 是否登记在指定 section 内。
// 注意不能做全文件包含判断：details 段存在不代表已安装。
func acfSectionContains(content, section, workshopId string) bool {
	marker := "\"" + section + "\"\n\t{"
	idx := strings.Index(content, marker)
	if idx < 0 {
		return false
	}
	return strings.Contains(braceBody(content, idx+len(marker)-1), "\""+workshopId+"\"")
}

// braceBody 从开括号位置起做花括号配对，返回整段内容（含首尾括号）
func braceBody(content string, open int) string {
	depth := 0
	for i := open; i < len(content); i++ {
		switch content[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return content[open : i+1]
			}
		}
	}
	return content[open:]
}

// acfEntryBlock 生成与游戏写入风格一致的 tab 缩进登记块
func acfEntryBlock(workshopId string, fields [][2]string) string {
	var b strings.Builder
	b.WriteString("\t\t\"" + workshopId + "\"\n\t\t{\n")
	for _, f := range fields {
		b.WriteString("\t\t\t\"" + f[0] + "\"\t\t\"" + f[1] + "\"\n")
	}
	b.WriteString("\t\t}\n")
	return b.String()
}

// insertAcfSectionEntry 把登记块插入 ACF 指定 section 的开括号之后。
// section 不存在时原样返回（由调用方决定是否需要骨架兜底）。
func insertAcfSectionEntry(content, section, entry string) string {
	marker := "\"" + section + "\"\n\t{"
	idx := strings.Index(content, marker)
	if idx < 0 {
		return content
	}
	at := idx + len(marker)
	return content[:at] + "\n" + entry + content[at:]
}

// dirSize 统计 ACF 同级 content/322330/<id> 目录的字节数，目录缺失返回 0
func dirSize(contentRoot, workshopId string) int64 {
	var total int64
	_ = filepath.Walk(filepath.Join(contentRoot, "content", "322330", workshopId), func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}
