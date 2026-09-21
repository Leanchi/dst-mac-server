package mod

import "strings"

// cdnZipFilename Steam 对 CDN-zip 型老模组的 filename 标识（2017 前的旧上传管线产物）。
// 实测对照（2026-09-21）：该类型模组游戏内下载走 steamclient ISteamHTTP，box64 下恒超时，
// 且 ACF/模组登记均无法绕过认证；manifest 型（filename 为空或目录名）走内容服务器通道，正常。
const cdnZipFilename = "mod_publish_data_file.zip"

// cdnZipWarningDesc 注入搜索结果 desc 的提示行：前端现成展示 desc，无需改 UI 即可见。
const cdnZipWarningDesc = "\n\n⚠️ 此模组为 CDN-zip 旧版封装：Apple Silicon（box64）环境下游戏无法自动下载认证，需手动本地化才能加载，不建议添加。"

// isCdnZipMod 判断 Steam API 条目（QueryFiles/GetDetails 的 filename 字段）是否为 CDN-zip 型
func isCdnZipMod(filename string) bool {
	return strings.Contains(filename, cdnZipFilename)
}

// applyCdnZipPolicy 对搜索结果应用 CDN-zip 策略：标记、desc 注入警告、按需过滤。
// 过滤仅作用于当页（Steam 返回的 Total 保持原值，为近似计数）。
func applyCdnZipPolicy(list []ModInfo, excludeCdnZip bool) []ModInfo {
	result := make([]ModInfo, 0, len(list))
	for _, mod := range list {
		if mod.CdnZip {
			if excludeCdnZip {
				continue
			}
			mod.Desc += cdnZipWarningDesc
		}
		result = append(result, mod)
	}
	return result
}
