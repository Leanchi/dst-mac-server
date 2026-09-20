package mod

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnabledWorkshopIds(t *testing.T) {
	content := `return {
  ["workshop-1185229307"]={ configuration_options={ ["x"]=1 }, enabled=true },
  ["workshop-1898292532"]={ enabled=true },
  ["workshop-1185229307"]={ enabled=true },
}`
	ids := enabledWorkshopIds(content)
	if len(ids) != 2 || ids[0] != "1185229307" || ids[1] != "1898292532" {
		t.Fatalf("应去重并保序提取 2 个 ID，实际: %v", ids)
	}
	if got := enabledWorkshopIds("return {}"); len(got) != 0 {
		t.Fatalf("空 modoverrides 应返回空，实际: %v", got)
	}
}

func TestEnsureAcfEntry(t *testing.T) {
	t.Run("文件不存在时创建含登记的骨架", func(t *testing.T) {
		acf := filepath.Join(t.TempDir(), "appworkshop_322330.acf")
		if err := ensureAcfEntry(acf, "123456789"); err != nil {
			t.Fatal(err)
		}
		assertAcfValid(t, acf, "123456789")
	})

	t.Run("已有 ACF 时两段插入且括号平衡", func(t *testing.T) {
		acf := filepath.Join(t.TempDir(), "appworkshop_322330.acf")
		existing := "\"AppWorkshop\"\n{\n\t\"appid\"\t\t\"322330\"\n" +
			"\t\"WorkshopItemsInstalled\"\n\t{\n\t\t\"111\"\n\t\t{\n\t\t}\n\t}\n" +
			"\t\"WorkshopItemDetails\"\n\t{\n\t\t\"111\"\n\t\t{\n\t\t}\n\t}\n}\n"
		if err := os.WriteFile(acf, []byte(existing), 0644); err != nil {
			t.Fatal(err)
		}
		if err := ensureAcfEntry(acf, "222"); err != nil {
			t.Fatal(err)
		}
		assertAcfValid(t, acf, "111")
		assertAcfValid(t, acf, "222")
	})

	t.Run("已登记则跳过不改写", func(t *testing.T) {
		acf := filepath.Join(t.TempDir(), "appworkshop_322330.acf")
		existing := "\"AppWorkshop\"\n{\n\t\"appid\"\t\t\"322330\"\n" +
			"\t\"WorkshopItemsInstalled\"\n\t{\n\t\t\"111\"\n\t\t{\n\t\t\t\"manifest\"\t\t\"777\"\n\t\t}\n\t}\n" +
			"\t\"WorkshopItemDetails\"\n\t{\n\t\t\"111\"\n\t\t{\n\t\t\t\"manifest\"\t\t\"777\"\n\t\t}\n\t}\n}\n"
		if err := os.WriteFile(acf, []byte(existing), 0644); err != nil {
			t.Fatal(err)
		}
		before, _ := os.ReadFile(acf)
		if err := ensureAcfEntry(acf, "111"); err != nil {
			t.Fatal(err)
		}
		after, _ := os.ReadFile(acf)
		if string(before) != string(after) {
			t.Fatal("已登记的模组不应改写 ACF")
		}
		// 前缀 ID 不应误判已登记（"11" ≠ "111"）
		if err := ensureAcfEntry(acf, "11"); err != nil {
			t.Fatal(err)
		}
		assertAcfValid(t, acf, "11")
	})

	t.Run("details-only 时必须补 installed 段", func(t *testing.T) {
		// 复刻实测病灶：CDN zip 型老模组只有 details 登记，游戏视为未安装
		acf := filepath.Join(t.TempDir(), "appworkshop_322330.acf")
		existing := "\"AppWorkshop\"\n{\n\t\"appid\"\t\t\"322330\"\n" +
			"\t\"WorkshopItemsInstalled\"\n\t{\n\t\t\"999\"\n\t\t{\n\t\t}\n\t}\n" +
			"\t\"WorkshopItemDetails\"\n\t{\n\t\t\"661\"\n\t\t{\n\t\t\t\"manifest\"\t\t\"-1\"\n\t\t}\n\t}\n}\n"
		if err := os.WriteFile(acf, []byte(existing), 0644); err != nil {
			t.Fatal(err)
		}
		if err := ensureAcfEntry(acf, "661"); err != nil {
			t.Fatal(err)
		}
		// installed 段必须新增 661，details 段保留原有条目
		assertAcfValid(t, acf, "661")
		// 999 原本就只在 installed 段，不应被动到
		content, _ := os.ReadFile(acf)
		if !strings.Contains(extractSection(t, string(content), "WorkshopItemsInstalled"), "\"999\"") {
			t.Fatal("原有 installed 段条目不应被破坏")
		}
	})
}

// assertAcfValid 校验：ID 在两段 section 中各出现、全文花括号平衡
func assertAcfValid(t *testing.T, acfPath, id string) {
	t.Helper()
	content, err := os.ReadFile(acfPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	installed := extractSection(t, s, "WorkshopItemsInstalled")
	details := extractSection(t, s, "WorkshopItemDetails")
	if !strings.Contains(installed, "\""+id+"\"") || !strings.Contains(details, "\""+id+"\"") {
		t.Fatalf("ID %s 应同时在两段 section 中登记", id)
	}
	if strings.Count(s, "{") != strings.Count(s, "}") {
		t.Fatalf("ACF 花括号不平衡: { %d vs } %d", strings.Count(s, "{"), strings.Count(s, "}"))
	}
}

func extractSection(t *testing.T, content, section string) string {
	t.Helper()
	marker := "\"" + section + "\"\n\t{"
	idx := strings.Index(content, marker)
	if idx < 0 {
		t.Fatalf("缺少 section %s", section)
	}
	// 从 section 开括号起做花括号配对截取整段
	depth, start := 0, idx+len(marker)-1
	for i := start; i < len(content); i++ {
		switch content[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return content[start : i+1]
			}
		}
	}
	t.Fatalf("section %s 花括号不闭合", section)
	return ""
}
