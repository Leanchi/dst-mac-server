package steam

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DepotDownloader 封装：替代 steamcmd 完成游戏安装/更新与创意工坊模组下载。
// 仅在非 Windows 分支被调用（调用点已是 Linux 分支），包内不做平台判断。

const (
	// dstAppID 饥荒联机版专用服务器 Steam AppID
	dstAppID = "343050"
	// betaBranch 正式服测试分支（上游 steamcmd 的 -beta updatebeta 语义）
	betaBranch = "updatebeta"
	// envKey 环境变量名，优先级最高的 DepotDownloader 可执行文件路径
	envKey = "DEPOT_DOWNLOADER"
	// defaultPath 容器镜像内的默认安装位置
	defaultPath = "/opt/DepotDownloader/DepotDownloader"
	// executableName 复用面板 Steamcmd 配置字段时的可执行文件名
	executableName = "DepotDownloader"
)

// ResolvePath 解析 DepotDownloader 可执行文件路径。
// 优先级：环境变量 DEPOT_DOWNLOADER > <steamcmdDir>/DepotDownloader > /opt/DepotDownloader/DepotDownloader
// steamcmdDir 为面板集群配置的 Steamcmd 字段（复用上游字段，镜像内默认即指向 DepotDownloader 目录）。
func ResolvePath(steamcmdDir string) string {
	if p := strings.TrimSpace(os.Getenv(envKey)); p != "" {
		return p
	}
	if steamcmdDir != "" {
		return filepath.Join(steamcmdDir, executableName)
	}
	return defaultPath
}

// DownloadApp 下载/更新 DST 服务端（增量下载，仅拉取变更 chunk）。
// beta 为 true 时拉取 updatebeta 分支；steamcmdDir 用于解析 DepotDownloader 路径。
func DownloadApp(dir string, beta bool, steamcmdDir string) error {
	args := buildAppArgs(dir, beta)
	return run(ResolvePath(steamcmdDir), args)
}

// DownloadPubfile 下载创意工坊物品（等同 steamcmd +workshop_download_item 322330 <modId>）。
// 产物直接落盘到 dir，满足面板 <Mod_download_path>/steamapps/workshop/content/322330/<modId> 路径契约。
func DownloadPubfile(modID string, dir string, steamcmdDir string) error {
	args := buildPubfileArgs(modID, dir)
	return run(ResolvePath(steamcmdDir), args)
}

// buildAppArgs 构造游戏下载参数数组（不经 shell 拼接，天然免转义）
func buildAppArgs(dir string, beta bool) []string {
	args := []string{"-app", dstAppID, "-os", "linux", "-osarch", "64", "-dir", dir}
	if beta {
		args = append(args, "-branch", betaBranch)
	}
	return append(args, "-validate")
}

// buildPubfileArgs 构造创意工坊物品下载参数数组
func buildPubfileArgs(modID string, dir string) []string {
	return []string{"-pubfile", modID, "-dir", dir}
}

// run 执行 DepotDownloader，stdout/stderr 合流转发到面板日志。
// 沿用上游 steamcmd 无超时行为，不设执行超时。
func run(execPath string, args []string) error {
	log.Println("正在执行 DepotDownloader command:", execPath, strings.Join(args, " "))

	// context.Background：当前无超时需求，CommandContext 为后续接入超时留口
	cmd := exec.CommandContext(context.Background(), execPath, args...)
	cmd.Stdout = logWriter{}
	cmd.Stderr = logWriter{}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("DepotDownloader 执行失败: %w", err)
	}
	return nil
}

// logWriter 把子进程输出逐块写入面板日志
type logWriter struct{}

func (logWriter) Write(p []byte) (int, error) {
	log.Print(string(p))
	return len(p), nil
}
