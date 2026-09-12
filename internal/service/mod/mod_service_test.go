package mod

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// TestWorkshopModPath 面板产物路径契约：后续拷入集群 mods 目录的逻辑依赖该路径
func TestWorkshopModPath(t *testing.T) {
	got := workshopModPath("/data/mod", "362175979")
	want := "/data/mod/steamapps/workshop/content/322330/362175979"
	if got != want {
		t.Fatalf("路径契约不符\nwant: %s\ngot:  %s", want, got)
	}
}

// writeTestZip 在 path 处生成包含给定文件的 zip
func writeTestZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("创建zip失败: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("写入zip条目失败: %v", err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatalf("写入zip内容失败: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("关闭zip失败: %v", err)
	}
}

func TestExtractModZipUnzipsAndDeletes(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "365119238.zip")
	writeTestZip(t, zipPath, map[string]string{
		"modinfo.lua":     `name = "test mod"`,
		"scripts/mod.lua": `-- mod script`,
	})

	s := &ModService{}
	s.extractModZip(dir)

	if _, err := os.Stat(filepath.Join(dir, "modinfo.lua")); err != nil {
		t.Fatalf("modinfo.lua 应被解压到目录根部: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "scripts", "mod.lua")); err != nil {
		t.Fatalf("子目录文件应被解压: %v", err)
	}
	if _, err := os.Stat(zipPath); !os.IsNotExist(err) {
		t.Fatalf("解压成功后 zip 应被删除")
	}
}

func TestExtractModZipSkipsWhenModinfoExists(t *testing.T) {
	dir := t.TempDir()
	modinfoPath := filepath.Join(dir, "modinfo.lua")
	if err := os.WriteFile(modinfoPath, []byte(`name = "already here"`), 0o644); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(dir, "some.zip")
	writeTestZip(t, zipPath, map[string]string{"modinfo.lua": `name = "from zip"`})

	s := &ModService{}
	s.extractModZip(dir)

	if _, err := os.Stat(zipPath); err != nil {
		t.Fatalf("已有 modinfo.lua 时不应处理 zip: %v", err)
	}
	data, err := os.ReadFile(modinfoPath)
	if err != nil || string(data) != `name = "already here"` {
		t.Fatalf("已有 modinfo.lua 不应被覆盖: %q %v", string(data), err)
	}
}

func TestExtractModZipKeepsZipWhenExtractionFails(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "broken.zip")
	if err := os.WriteFile(zipPath, []byte("not a zip file"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &ModService{}
	s.extractModZip(dir)

	if _, err := os.Stat(zipPath); err != nil {
		t.Fatalf("解压失败时不应删除 zip")
	}
}
