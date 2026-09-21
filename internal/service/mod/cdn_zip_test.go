package mod

import "testing"

func TestIsCdnZipMod(t *testing.T) {
	cases := []struct {
		filename string
		want     bool
	}{
		{"mod_publish_data_file.zip", true},                 // 实测 CDN-zip 型（661253977/1898181913）
		{"workshop/mod_publish_data_file.zip", true},        // 带路径前缀的变体
		{"", false},                                         // manifest 型：filename 为空
		{"Campfire Respawn", false},                         // manifest 型：目录名
		{"some_mod_v2.zip", false},                          // 其他 zip 名不误伤
	}
	for _, c := range cases {
		if got := isCdnZipMod(c.filename); got != c.want {
			t.Fatalf("isCdnZipMod(%q) = %v, 期望 %v", c.filename, got, c.want)
		}
	}
}

func TestApplyCdnZipPolicy(t *testing.T) {
	list := []ModInfo{
		{ID: "1", Desc: "正常模组"},
		{ID: "2", Desc: "CDN-zip 老模组", CdnZip: true},
		{ID: "3", Desc: "另一个正常模组"},
	}

	t.Run("默认过滤", func(t *testing.T) {
		got := applyCdnZipPolicy(list, true)
		if len(got) != 2 || got[0].ID != "1" || got[1].ID != "3" {
			t.Fatalf("应过滤 CDN-zip 项，实际: %v", got)
		}
	})

	t.Run("关闭过滤时保留并注入警告", func(t *testing.T) {
		got := applyCdnZipPolicy(list, false)
		if len(got) != 3 {
			t.Fatalf("应保留全部 3 项，实际 %d 项", len(got))
		}
		if got[1].Desc == "CDN-zip 老模组" || got[1].Desc == "" {
			t.Fatal("CDN-zip 项的 desc 应注入警告文案")
		}
		if got[0].Desc != "正常模组" {
			t.Fatal("正常项的 desc 不应被改动")
		}
	})
}
