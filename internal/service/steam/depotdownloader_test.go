package steam

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// requirePosixSkip 非 POSIX 平台跳过（假二进制用 shell 脚本实现）
func requirePosixSkip(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("假二进制依赖 shell 脚本，Windows 跳过")
	}
}

// writeFakeDD 生成一个假 DepotDownloader：把收到的参数逐行记录到 DD_TEST_RECORD_FILE 指定文件
func writeFakeDD(t *testing.T, exitCode int) string {
	t.Helper()
	requirePosixSkip(t)

	script := filepath.Join(t.TempDir(), "DepotDownloader")
	content := fmt.Sprintf("#!/bin/sh\nfor a in \"$@\"; do printf '%%s\\n' \"$a\" >> \"$DD_TEST_RECORD_FILE\"; done\nexit %d\n", exitCode)
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("写入假二进制失败: %v", err)
	}
	return script
}

// recordedArgs 读取假二进制记录的参数
func recordedArgs(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(os.Getenv("DD_TEST_RECORD_FILE"))
	if err != nil {
		t.Fatalf("读取假二进制参数记录失败: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

func TestResolvePath(t *testing.T) {
	t.Run("环境变量优先", func(t *testing.T) {
		t.Setenv(envKey, "/custom/dd")
		if got := ResolvePath("/data/steamcmd"); got != "/custom/dd" {
			t.Fatalf("期望环境变量路径生效, got %q", got)
		}
	})

	t.Run("环境变量空白视为未设置", func(t *testing.T) {
		t.Setenv(envKey, "   ")
		if got := ResolvePath("/data/steamcmd"); got != filepath.Join("/data/steamcmd", "DepotDownloader") {
			t.Fatalf("期望集群 Steamcmd 目录拼接生效, got %q", got)
		}
	})

	t.Run("无环境变量用集群Steamcmd目录", func(t *testing.T) {
		t.Setenv(envKey, "")
		if got := ResolvePath("/data/steamcmd"); got != "/data/steamcmd/DepotDownloader" {
			t.Fatalf("期望 /data/steamcmd/DepotDownloader, got %q", got)
		}
	})

	t.Run("都为空用默认路径", func(t *testing.T) {
		t.Setenv(envKey, "")
		if got := ResolvePath(""); got != "/opt/DepotDownloader/DepotDownloader" {
			t.Fatalf("期望默认路径, got %q", got)
		}
	})
}

func TestBuildAppArgs(t *testing.T) {
	dir := "/app/dst-dedicated-server"

	t.Run("非beta", func(t *testing.T) {
		want := []string{"-app", "343050", "-os", "linux", "-osarch", "64", "-dir", dir, "-validate"}
		got := buildAppArgs(dir, false)
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Fatalf("参数数组不符\nwant: %v\ngot:  %v", want, got)
		}
	})

	t.Run("beta带updatebeta分支", func(t *testing.T) {
		want := []string{"-app", "343050", "-os", "linux", "-osarch", "64", "-dir", dir + "-beta", "-branch", "updatebeta", "-validate"}
		got := buildAppArgs(dir+"-beta", true)
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Fatalf("参数数组不符\nwant: %v\ngot:  %v", want, got)
		}
	})
}

func TestBuildPubfileArgs(t *testing.T) {
	dir := "/data/mod/steamapps/workshop/content/322330/362175979"
	want := []string{"-pubfile", "362175979", "-dir", dir}
	got := buildPubfileArgs("362175979", dir)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("参数数组不符\nwant: %v\ngot:  %v", want, got)
	}
}

func TestDownloadAppRunsFakeBinary(t *testing.T) {
	requirePosixSkip(t)

	record := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("DD_TEST_RECORD_FILE", record)
	t.Setenv(envKey, writeFakeDD(t, 0))

	dir := t.TempDir()
	if err := DownloadApp(dir, true, "/ignored/steamcmd"); err != nil {
		t.Fatalf("DownloadApp 执行失败: %v", err)
	}

	want := []string{"-app", "343050", "-os", "linux", "-osarch", "64", "-dir", dir, "-branch", "updatebeta", "-validate"}
	if got := recordedArgs(t); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("实际执行参数不符\nwant: %v\ngot:  %v", want, got)
	}
}

func TestDownloadPubfileRunsFakeBinary(t *testing.T) {
	requirePosixSkip(t)

	record := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("DD_TEST_RECORD_FILE", record)
	t.Setenv(envKey, writeFakeDD(t, 0))

	dir := filepath.Join(t.TempDir(), "steamapps", "workshop", "content", "322330", "362175979")
	if err := DownloadPubfile("362175979", dir, ""); err != nil {
		t.Fatalf("DownloadPubfile 执行失败: %v", err)
	}

	want := []string{"-pubfile", "362175979", "-dir", dir}
	if got := recordedArgs(t); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("实际执行参数不符\nwant: %v\ngot:  %v", want, got)
	}
}

func TestDownloadFailsWhenBinaryExitsNonZero(t *testing.T) {
	requirePosixSkip(t)

	record := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("DD_TEST_RECORD_FILE", record)
	t.Setenv(envKey, writeFakeDD(t, 3))

	if err := DownloadApp(t.TempDir(), false, ""); err == nil {
		t.Fatal("期望假二进制非零退出时返回错误")
	}
}
