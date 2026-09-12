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

func (u LinuxUpdate) Update(clusterName string) error {
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

	log.Println("正在更新游戏", "cluster: ", clusterName, "dir: ", dstInstallDir, "beta: ", beta)
	err = steam.DownloadApp(dstInstallDir, beta, config.Steamcmd)
	if err == nil {
		return nil
	}

	log.Println("更新游戏失败，清理 Steam 下载缓存后重试一次", "cluster: ", clusterName, "error: ", err)
	cleanupSteamDownloadCache(config)
	retryErr := steam.DownloadApp(dstInstallDir, beta, config.Steamcmd)
	if retryErr != nil {
		return retryErr
	}
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
