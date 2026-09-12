package update

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"dst-admin-go/internal/service/dstConfig"
)

// stubConfig 便于单测注入集群配置
type stubConfig struct{ cfg dstConfig.DstConfig }

func (s stubConfig) GetDstConfig(string) (dstConfig.DstConfig, error) {
	return s.cfg, nil
}

func (s stubConfig) SaveDstConfig(string, dstConfig.DstConfig) error {
	return nil
}

// writeFakeDD 生成假 DepotDownloader：每次调用计数 + 记录参数；
// 前 DD_TEST_FAIL_UNTIL 次调用以非零码退出（模拟下载失败）。
func writeFakeDD(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("假二进制依赖 shell 脚本，Windows 跳过")
	}

	script := filepath.Join(t.TempDir(), "DepotDownloader")
	content := `#!/bin/sh
n=$(cat "$DD_TEST_COUNT_FILE" 2>/dev/null || echo 0)
n=$((n+1))
echo "$n" > "$DD_TEST_COUNT_FILE"
line=
for a in "$@"; do line="$line$a "; done
printf '%s\n' "$line" >> "$DD_TEST_CALLS_FILE"
if [ -n "$DD_TEST_FAIL_UNTIL" ] && [ "$n" -le "$DD_TEST_FAIL_UNTIL" ]; then
  echo "simulated download failure" >&2
  exit 1
fi
exit 0
`
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("写入假二进制失败: %v", err)
	}
	return script
}

// setupFakeDDEnv 配置假二进制环境并返回参数记录文件路径
func setupFakeDDEnv(t *testing.T, failUntil string) string {
	t.Helper()
	record := filepath.Join(t.TempDir(), "calls.txt")
	count := filepath.Join(t.TempDir(), "count.txt")
	t.Setenv("DD_TEST_CALLS_FILE", record)
	t.Setenv("DD_TEST_COUNT_FILE", count)
	t.Setenv("DD_TEST_FAIL_UNTIL", failUntil)
	t.Setenv("DEPOT_DOWNLOADER", writeFakeDD(t))
	return record
}

// recordedCalls 逐行返回假二进制收到的参数（按空白归一化）
func recordedCalls(t *testing.T, record string) []string {
	t.Helper()
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("读取调用记录失败: %v", err)
	}
	var calls []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		calls = append(calls, strings.Join(strings.Fields(line), " "))
	}
	return calls
}

func TestLinuxUpdateBetaSemantics(t *testing.T) {
	record := setupFakeDDEnv(t, "")
	dstDir := t.TempDir()

	u := NewLinuxUpdate(stubConfig{cfg: dstConfig.DstConfig{
		Force_install_dir: dstDir,
		Beta:              1,
	}})
	if err := u.Update("cluster-1"); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}

	calls := recordedCalls(t, record)
	if len(calls) != 1 {
		t.Fatalf("期望调用 1 次, got %d: %v", len(calls), calls)
	}
	want := "-app 343050 -os linux -osarch 64 -dir " + dstDir + "-beta -branch updatebeta -validate"
	if calls[0] != want {
		t.Fatalf("beta 语义不符\nwant: %s\ngot:  %s", want, calls[0])
	}
}

func TestLinuxUpdateNonBetaSemantics(t *testing.T) {
	record := setupFakeDDEnv(t, "")
	dstDir := t.TempDir()

	u := NewLinuxUpdate(stubConfig{cfg: dstConfig.DstConfig{
		Force_install_dir: dstDir,
		Beta:              0,
	}})
	if err := u.Update("cluster-1"); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}

	calls := recordedCalls(t, record)
	if len(calls) != 1 {
		t.Fatalf("期望调用 1 次, got %d: %v", len(calls), calls)
	}
	want := "-app 343050 -os linux -osarch 64 -dir " + dstDir + " -validate"
	if calls[0] != want {
		t.Fatalf("非 beta 语义不符\nwant: %s\ngot:  %s", want, calls[0])
	}
}

func TestLinuxUpdateRetriesOnceAndCleansCache(t *testing.T) {
	record := setupFakeDDEnv(t, "1")
	dstDir := filepath.Join(t.TempDir(), "game-beta")

	// 预置 Steam 下载缓存，重试前应被清理
	cachePaths := []string{
		filepath.Join(dstDir, "steamapps", "downloading"),
		filepath.Join(dstDir, "steamapps", "temp"),
		filepath.Join(dstDir, "steamapps", "appmanifest_343050.acf"),
	}
	for _, p := range cachePaths {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	u := NewLinuxUpdate(stubConfig{cfg: dstConfig.DstConfig{
		Force_install_dir: strings.TrimSuffix(dstDir, "-beta"),
		Beta:              1,
	}})
	if err := u.Update("cluster-1"); err != nil {
		t.Fatalf("重试后应成功, got: %v", err)
	}
	calls := recordedCalls(t, record)
	if len(calls) != 2 {
		t.Fatalf("期望失败重试共调用 2 次, got %d: %v", len(calls), calls)
	}
	for _, p := range cachePaths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("缓存 %s 应已被清理", p)
		}
	}
}

func TestLinuxUpdateReturnsErrorWhenBothAttemptsFail(t *testing.T) {
	record := setupFakeDDEnv(t, "2")

	u := NewLinuxUpdate(stubConfig{cfg: dstConfig.DstConfig{
		Force_install_dir: t.TempDir(),
		Beta:              0,
	}})
	if err := u.Update("cluster-1"); err == nil {
		t.Fatal("两次都失败时应返回错误")
	}
	if calls := recordedCalls(t, record); len(calls) != 2 {
		t.Fatalf("期望共调用 2 次, got %d", len(calls))
	}
}
