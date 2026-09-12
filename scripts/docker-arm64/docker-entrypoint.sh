#!/bin/bash

# ARM64（Apple Silicon / linux/arm64）镜像入口脚本。
# /app/data 播种逻辑与 amd64 正式版（scripts/docker/docker-entrypoint.sh）对齐；
# 游戏安装/更新改用 DepotDownloader（.NET 8 arm64 原生，增量下载），
# 游戏本体（x86_64 ELF）经 box64 运行。

# 修正最大文件描述符数，部分docker版本给的默认值过高，会导致screen运行卡顿
ulimit -Sn 10000

# 游戏安装目录：默认 /app/dst-dedicated-server，可用环境变量 DST_DIR 覆盖
# （需与播种的 docker_dst_config 中 force_install_dir 保持一致）
steam_dst_server="${DST_DIR:-/app/dst-dedicated-server}"
data_dir='/app/data'
dd_bin='/opt/DepotDownloader/DepotDownloader'

# SKIP_GAME_INSTALL=1：跳过游戏首装、steamclient.so 就位与 chmod，直接启动面板。
# 用途：CI 冒烟测试（面板可独立起服验证）、调试 entrypoint、或只先用面板再手动安装游戏。
skip_game_install="${SKIP_GAME_INSTALL:-0}"

# ---------- 播种 /app/data（与 amd64 版 entrypoint 逻辑一致） ----------
# 确保持久化数据目录存在，并在其为空（例如首次挂载一个全新的空 volume）时
# 从镜像内置的默认值播种 dst_config 和管理员账户文件。幂等：仅缺失时拷入。
mkdir -p "$data_dir"
if [ ! -f "$data_dir/dst_config" ]; then
  cp /app/docker_dst_config.default "$data_dir/dst_config"
fi
if [ ! -f "$data_dir/password.txt" ]; then
  echo "username=admin" >> "$data_dir/password.txt"
  echo "password=123456" >> "$data_dir/password.txt"
  echo "displayName=admin" >> "$data_dir/password.txt"
  echo "photoURL=xxx" >> "$data_dir/password.txt"
fi
mkdir -p "$data_dir/backup"
mkdir -p "$data_dir/mod"
# 与 docker_dst_config 中的 persistent_storage_root=/app/data 和默认的
# cluster=Cluster_1 保持一致（游戏本体自己会在这之下再建一层 DoNotStarveTogether
# 目录），方便直接把已有存档挂载到这个固定路径上。
mkdir -p "$data_dir/DoNotStarveTogether/Cluster_1"

if [ "$skip_game_install" = "1" ]; then
  echo "SKIP_GAME_INSTALL=1：跳过游戏安装/steamclient 就位，直接启动面板"
  cd /app || exit 1
  exec ./dst-admin-go
fi

# ---------- 条件首装（修复上游半成品每次启动全量 validate 的问题） ----------
# appmanifest_343050.acf 是 Steam 安装完成标记：存在即视为已安装，秒级跳过；
# DST_FORCE_UPDATE=1 可强制走一次增量更新（-validate 只校验差异）。
appmanifest="$steam_dst_server/steamapps/appmanifest_343050.acf"
if [ ! -f "$appmanifest" ] || [ "$DST_FORCE_UPDATE" = "1" ]; then
  echo "未检测到游戏本体（$appmanifest 缺失）或 DST_FORCE_UPDATE=1，开始通过 DepotDownloader 安装/更新 DST 服务端..."
  retry=1
  while [ "$retry" -le 3 ]; do
    if "$dd_bin" -app 343050 -os linux -osarch 64 -dir "$steam_dst_server" -validate; then
      break
    fi
    echo "DepotDownloader 第 ${retry} 次尝试失败"
    if [ "$retry" -ge 3 ]; then
      echo "DST 服务端安装失败（已重试 3 次），请检查网络（需可访问 Steam CDN）后重启容器重试"
      exit 1
    fi
    sleep 3
    retry=$((retry + 1))
  done
else
  echo "检测到游戏本体已安装（$appmanifest 存在），跳过安装。如需强制更新请设置 DST_FORCE_UPDATE=1"
fi

# ---------- steamclient.so 就位（游戏运行时连接 Steam 网络依赖） ----------
# 无需解包 steamcmd tarball：DST Linux 发行包（depot 1006）自带 steamclient.so。
# 实测（2026-09-13 depot 1006 manifest 6403079453）：游戏目录根的 steamclient.so 是
# ELF32（给 bin/ 下的 32 位服务端用），bin64 的 64 位服务端需要 linux64/ 下的 ELF64 版，
# 因此优先找 linux64/，并校验 ELF class=2（ELF64）后才拷入 ~/.steam/sdk64/。
# 找不到合适的仅告警不退出（游戏没装完时下次启动会再次尝试）。
home_dir="${HOME:-/root}"
mkdir -p "$home_dir/.steam/sdk64"

# 读 ELF 头 offset 4 的 class 字节：1=ELF32，2=ELF64
is_elf64() {
  [ -f "$1" ] && [ "$(od -An -tu1 -j4 -N1 "$1" | tr -d '[:space:]')" = "2" ]
}

steamclient_dst="$home_dir/.steam/sdk64/steamclient.so"
for candidate in "$steam_dst_server/linux64/steamclient.so" "$steam_dst_server/steamclient.so"; do
  if is_elf64 "$candidate"; then
    cp -f "$candidate" "$steamclient_dst"
    echo "已就位 steamclient.so：$candidate -> $steamclient_dst"
    break
  fi
done
if [ ! -f "$steamclient_dst" ]; then
  echo "警告：未找到可用的 64 位 steamclient.so（$steam_dst_server 下），游戏安装完成后重启容器会自动就位"
fi

# ---------- 游戏二进制可执行位（zip 解包不保留 exec bit，幂等兜底） ----------
game_bin="$steam_dst_server/bin64/dontstarve_dedicated_server_nullrenderer_x64"
if [ -f "$game_bin" ]; then
  chmod +x "$game_bin"
else
  echo "警告：未找到游戏二进制 $game_bin，游戏可能尚未安装完成"
fi

cd /app || exit 1
exec ./dst-admin-go
