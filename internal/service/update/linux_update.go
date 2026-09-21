package update

import (
	"dst-admin-go/internal/service/dstConfig"
	"dst-admin-go/internal/service/steam"
	"log"
	"os"
	"path/filepath"
)

type LinuxUpdate struct {
	dstConfig dstConfig.Config
}

func NewLinuxUpdate(dstConfig dstConfig.Config) *LinuxUpdate {
	return &LinuxUpdate{
		dstConfig: dstConfig,
	}
}

func (u LinuxUpdate) Update(clusterName string, isDelete bool) error {
	config, err := u.dstConfig.GetDstConfig(clusterName)
	if err != nil {
		return err
	}

	// 上游语义：beta 平行目录（<dir>-beta）， DepotDownloader 拉取 updatebeta 分支
	dstInstallDir := config.Force_install_dir
	if config.Beta == 1 {
		dstInstallDir += "-beta"
	}
	beta := config.Beta == 1

	if isDelete {
		if err := cleanGameDir(dstInstallDir); err != nil {
			return err
		}
	}

	log.Println("正在更新游戏", "cluster: ", clusterName, "dir: ", dstInstallDir, "beta: ", beta, "isDelete: ", isDelete)
	err = steam.DownloadApp(dstInstallDir, beta, config.Steamcmd)
	if err == nil {
		return ensureExecutable(dstInstallDir)
	}

	log.Println("更新游戏失败，清理 Steam 下载缓存后重试一次", "cluster: ", clusterName, "error: ", err)
	cleanupSteamDownloadCache(config)
	retryErr := steam.DownloadApp(dstInstallDir, beta, config.Steamcmd)
	if retryErr != nil {
		return retryErr
	}
	return ensureExecutable(dstInstallDir)
}

// ensureExecutable 补齐服务器二进制的可执行位。
// DepotDownloader 下载不保留 exec bit（实测「删除并更新」全量重装后二进制变
// 0644，box64 报 "is not an executable file" 直接拒启）；entrypoint 首装有
// 同样的 chmod 兜底，此处覆盖面板更新路径（增量/全量均幂等）。
func ensureExecutable(dstInstallDir string) error {
	bins := []string{
		filepath.Join(dstInstallDir, "bin64", "dontstarve_dedicated_server_nullrenderer_x64"),
		filepath.Join(dstInstallDir, "bin64", "dontstarve_dedicated_server_nullrenderer_x64_luajit"),
		filepath.Join(dstInstallDir, "bin", "dontstarve_dedicated_server_nullrenderer"),
	}
	for _, bin := range bins {
		info, err := os.Stat(bin)
		if err != nil {
			continue // 该变体未安装则跳过（幂等）
		}
		if info.Mode()&0100 != 0 {
			continue
		}
		if err := os.Chmod(bin, 0755); err != nil {
			return err
		}
		log.Println("已补可执行位", bin)
	}
	return nil
}

// cleanGameDir 清空游戏目录后全量重装（前端「删除并更新」语义）。
// 只删游戏本体文件，保留三类非游戏资产：
//   - ugc_mods/：游戏的创意工坊模组缓存（重建要靠不稳的联网下载）
//   - mods/：dedicated_server_mods_setup.lua 等面板管理的清单
//   - steamclient.so：容器 entrypoint 播种的 Steam 组件
func cleanGameDir(dir string) error {
	preserve := map[string]bool{
		"ugc_mods":      true,
		"mods":          true,
		"steamclient.so": true,
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if preserve[e.Name()] {
			log.Println("删除并更新：保留", filepath.Join(dir, e.Name()))
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	log.Println("删除并更新：游戏目录已清空（保留 ugc_mods/mods/steamclient.so）", dir)
	return nil
}

func cleanupSteamDownloadCache(config dstConfig.DstConfig) {
	dstInstallDir := config.Force_install_dir
	if config.Beta == 1 {
		dstInstallDir += "-beta"
	}
	paths := []string{
		filepath.Join(dstInstallDir, "steamapps", "downloading"),
		filepath.Join(dstInstallDir, "steamapps", "temp"),
		filepath.Join(dstInstallDir, "steamapps", "appmanifest_343050.acf"),
	}
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil {
			log.Println("清理 Steam 下载缓存失败", "path: ", path, "error: ", err)
		}
	}
}
