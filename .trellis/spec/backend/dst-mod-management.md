# DST 模组管线（三层结构与不可违反的契约）

> 来源：2026-09-15 UGC 模组"删不掉"与 ODPF 噪音两轮 debug 的根因沉淀。
> 修改任何模组相关代码前必读。

---

## 三层结构（谁决定什么）

| 层 | 文件/位置 | 谁写入 | 语义 |
|----|-----------|--------|------|
| **启用清单（唯一加载契约）** | `<存档>/<cluster>/<shard>/modoverrides.lua` | 面板「世界配置」保存时重写（`gameConfig.SaveConfig`） | 游戏启动**只加载这里列出的模组**。删/禁模组的最终效果以它为准 |
| **预下载清单** | `<游戏目录>/mods/dedicated_server_mods_setup.lua` | 同上（`SaveConfig` 同步重写） | 启动时游戏确保这些模组已下载，**缺失会联网下载，失败报 ODPF 错**（不阻塞加载）。与启用清单不一致 → 每次启动 ODPF 噪音 + 拖慢启动 |
| **游戏缓存 + 登记** | `<游戏目录>/ugc_mods/<cluster>/<shard>/content/322330/<id>/` + 同级 `appworkshop_322330.acf` | **游戏自己**下载时写入/重写 | 游戏实际加载的文件；ACF 是累积登记（游戏从不清理旧条目，只会追加/重写） |

面板另有自己的模组备份库：`/app/data/mod/steamapps/workshop/content/322330/<id>/`（DepotDownloader 下载，与游戏加载无关，仅作缓存/补拷来源）。

---

## 不可违反的契约

### C1. modoverrides 是唯一真相
判断"世界会用哪些模组"永远读 modoverrides.lua，不要读 setup.lua（预下载）或 ACF（缓存登记）。UI 上任何"启用/禁用"操作必须以重写 modoverrides 收尾。

### C2. 游戏会重建缓存与登记
删除 ugc_mods 文件或 ACF 条目只是**清缓存**——游戏启动时会按 setup.lua/世界需求重新下载并把条目**写回 ACF**。任何"删除后不再出现"的产品预期都不成立，除非同时把该模组从 modoverrides 与 setup.lua 移除。UGC 删除接口的成功文案必须传达这一点。

### C3. 每个世界（shard）各持一份缓存与登记
`ugc_mods/<cluster>/Master/` 与 `ugc_mods/<cluster>/Caves/` 各有独立的模组目录和 `appworkshop_322330.acf`。**删除/清理必须遍历全部 shard**（目录层级：`ugc_mods/<cluster>/<shard>/...`，注意 cluster 与 shard 是两层，遍历少一层会静默落空且接口仍返回成功——已在此处踩坑两次）。

### C4. 游戏识别模组目录的依据是 ACF 登记而非 WorkshopID.txt
（2026-09-20 勘误：实测 4 个启动即被 "already have" 识别的模组目录均无 WorkshopID.txt，
但有完整 ACF 登记——识别依据是 `appworkshop_322330.acf` 的 `WorkshopItemsInstalled` 段。）
DST 新版模组缓存路径为 `content/322330/<id>`（含 modinfo.lua 在目录根）。
从备份库播种/拷贝模组到 ugc_mods 时：目录 + ACF 两段登记（`WorkshopItemsInstalled` 与
`WorkshopItemDetails`）缺一不可，WorkshopID.txt 可写可不写。manifest 未知时登记 `-1`，
游戏如据此判定需更新会自行重下，更新失败不影响本次加载。

### C5. 模组产物路径契约（面板 DB ↔ 缓存）
面板启用模组时从缓存读 `modinfo.lua`，缓存路径
`<Mod_download_path>/steamapps/workshop/content/322330/<modId>/modinfo.lua`
是全管线契约（`workshopModPath` 单一事实源），任何改动必须同步 `DeleteMod`/`UpdateAllModInfos`/`AddModInfo` 消费方。

---

## 已知行为（非 bug，勿重复"修复"）

- **UGC 列表（`/api/mod/ugc/acf`）读 ACF**：显示的是缓存登记（含历史条目）。游戏启动会写回已删条目——这是 C2 的自然结果。
- **模组「禁用」**：上游语义为从启用清单移除；DB 记录仍在（setup.lua 写入来源是 DB 订阅列表）。「禁用后列表消失」是上游缺陷，修复时需区分 DB 订阅与启用两个概念。
- **启动早期日志为空**：游戏启动 5-45 秒后才写 server_log.txt（box64 冷启动更久），前端日志页此前的空白是等待而非卡死；前端启动确认窗口已放宽为 90s（`d=async(m,v,b=90)`）。
- **偶发无声死亡**：box64 下世界可能在启动中后期无日志死亡（无 OOM、无优雅关闭痕迹）。遇此直接重新启动观察；连续复现才需取证（screen Dead 残留会被启动前 `screen -wipe` 清理）。

---

## 启动前播种（SeedUgcCache 契约）

box64/steamclient 下游戏启动时的创意工坊自下载慢且不稳（`ODPF failed entirely` /
`DownloadServerMods timed out`），缺装模组时游戏带着残缺模组集开服，迟到下载下次重启才生效。
面板在每次启动世界前执行 `ModService.SeedUgcCache`（`internal/service/mod/ugc_seed.go`，
由 `GameHandler.Start/StartAll` 调用）：

- 清单来源 = 该世界 modoverrides.lua 的启用集合（C1）
- 逐世界（shard）独立播种（C3）：目标 `<ugc>/<cluster>/<shard>/content/322330/<id>`
- 目录缺 `modinfo.lua` → 从备份库拷贝（备份库为 zip 形态时先走 `extractModZip` 解压兜底）
- 目录在位后确保 ACF 两段登记存在（已登记则跳过不改写）
- 备份库也缺 → 跳过留给游戏自下载；任何失败只记日志，**不阻塞启动**

---

## 排查清单（模组问题先走这张表）

1. 世界实际加载了什么 → grep modoverrides.lua（不是 setup.lua）
2. 启动 ODPF 报错 → 对比 setup.lua 与 modoverrides 差集，重写 setup.lua 为启用集合
2.5. "只加载了部分模组" → 查 ugc 缓存缺目录或缺 ACF 登记（面板已内置启动前播种，冷备场景确认备份库有该模组）
3. "删了又出现" → C2：检查是否游戏运行中/之后启动过（写回），并确认 modoverrides 已无该模组
4. "拷进去的模组不生效" → C4：WorkshopID.txt 存在？目录层级 content/322330/<id>？
5. "删了列表还在" → C3：是否只清了一个 shard？ACF 两个 shard 都查

## 附：世界"莫名停止"排查（2026-09-15 实测）

世界运行 ~30 分钟后无声停止（日志止于 Sim paused，无错误）：**DST 官方防挂机行为**。
`pause_when_empty = true` 时默认 `IdleTimeout 1800s`（无人 30 分钟自动关服）。
- cluster.ini 写 `idle_timeout = false/0/大数字` **实测全部无效**（游戏版本 747465 恒打 1800s）
- 唯一解法：`pause_when_empty = false`（挂机模式，无人不暂停不自动关，代价是持续占用 CPU）
- 该开关即面板「世界配置 → 无人时暂停」，用户可自行切换
