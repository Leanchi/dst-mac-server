package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dst-admin-go/internal/pkg/utils/fileUtils"
	"dst-admin-go/internal/pkg/utils/zip"
)

func TestValidateBackupName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.zip"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	cases := []struct{ name, wantErr string }{
		{"ok.zip", ""},
		{"", "非法"},
		{"../etc/passwd", "非法"},
		{"a/b.zip", "非法"},
		{`a\b.zip`, "非法"},
		{"missing.zip", "不存在"},
	}
	for _, c := range cases {
		_, err := validateBackupName(dir, c.name)
		if c.wantErr == "" && err != nil {
			t.Fatalf("%q 应通过，实际报错: %v", c.name, err)
		}
		if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
			t.Fatalf("%q 应报含 %q 的错误，实际: %v", c.name, c.wantErr, err)
		}
	}
}

func TestRestoreSwapIn(t *testing.T) {
	// 构造源集群 → 打 zip
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "cluster.ini"), []byte("ini"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "Master", "save"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "Master", "modoverrides.lua"), []byte("lua"), 0644); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(t.TempDir(), "b.zip")
	if err := zip.Zip(src, zipPath); err != nil {
		t.Fatal(err)
	}

	t.Run("正常换入", func(t *testing.T) {
		dst := filepath.Join(t.TempDir(), "MyCluster")
		if err := os.MkdirAll(dst, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, "old.txt"), []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := restoreSwapIn(zipPath, dst); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dst, "cluster.ini")); err != nil {
			t.Fatal("恢复后应有 cluster.ini")
		}
		if _, err := os.Stat(filepath.Join(dst, "Master", "modoverrides.lua")); err != nil {
			t.Fatal("恢复后应有 modoverrides.lua")
		}
		if fileUtils.Exists(dst + ".restore-tmp") || fileUtils.Exists(dst + ".restore-old") {
			t.Fatal("临时目录应被清理")
		}
	})

	t.Run("损坏zip不动原存档", func(t *testing.T) {
		dst := filepath.Join(t.TempDir(), "MyCluster")
		if err := os.MkdirAll(dst, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, "old.txt"), []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
		badZip := filepath.Join(t.TempDir(), "bad.zip")
		if err := os.WriteFile(badZip, []byte("not a zip"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := restoreSwapIn(badZip, dst); err == nil {
			t.Fatal("损坏 zip 应报错")
		}
		if _, err := os.Stat(filepath.Join(dst, "old.txt")); err != nil {
			t.Fatal("原存档不应被破坏")
		}
	})
}
