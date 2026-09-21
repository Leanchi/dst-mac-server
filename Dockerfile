# =============================================================================
# （不使用 # syntax= 指令：该指令需从 Docker Hub 拉取 frontend 镜像，
#   在受限网络下可能失败；内置 BuildKit frontend 已覆盖本文件所需语法）
# =============================================================================
# dst-mac-server —— Apple Silicon (linux/arm64) DST 面板镜像
#
# stage 1 frontend: node:20（构建上游配套 web UI，dist 与后端 main 同源）
# stage 2 builder:  golang:1.24（构建机原生跑，交叉编译出 arm64 面板二进制）
# stage 3 runtime:  ubuntu:22.04 arm64
#   - dotnet-runtime-8.0（DepotDownloader 依赖）
#   - amd64 运行库（构建期装齐，供 box64 运行 x86_64 的 DST 服务端）
#   - box64（源码编译，ARM_DYNAREC=ON）
#   - DepotDownloader 3.4.0 linux-arm64 → /opt/DepotDownloader
#   - 面板二进制 + dist/ + static/ + 播种用默认配置
#
# 构建：docker buildx build --platform linux/arm64 -t dst-mac-server:dev .
# =============================================================================

# --------------------------------------------------------------- frontend ----
# 面板 web UI 源码在 companion 仓库（上游 CI 同款来源），现构建保证与后端 API 配套。
# 网络受限时可用 --build-arg NPM_REGISTRY=https://registry.npmmirror.com
FROM node:20-alpine AS frontend

ARG FRONTEND_REPO=carrot-hu23/dst-manage-web2
ARG FRONTEND_REF=main
ARG NPM_REGISTRY=https://registry.npmjs.org

WORKDIR /web
RUN wget -qO /tmp/web.tar.gz \
      "https://github.com/${FRONTEND_REPO}/archive/refs/heads/${FRONTEND_REF}.tar.gz" \
    && tar -xzf /tmp/web.tar.gz -C /web --strip-components=1 \
    && rm -f /tmp/web.tar.gz
# 本 fork 的源码级定制补丁：在上游源码解压后、构建前应用（比 sed 压缩产物稳定）。
# 补丁基于 FRONTEND_REF=main 生成；上游大改后需更新 scripts/frontend/ 下对应补丁。
COPY scripts/frontend/ /patches/
RUN apk add --no-cache patch \
    && patch -p0 < /patches/mod-search-cdnzip-filter.patch
RUN npm config set registry "${NPM_REGISTRY}" \
    && npm ci \
    && npm run build \
# 品牌化：界面产品名替换为本项目名。
# 仅改显示文案；指向上游的 GitHub 链接、LICENSE、日志路径等保持原样（GPL 合规）。
    && sed -i 's|document.title = "饥荒管理控制台"|document.title = "dst-mac-server · 饥荒管理控制台"|' /web/dist/index.html \
    && sed -i 's|饥荒联机版管理面板|dst-mac-server 面板|g' /web/dist/assets/*.js \
# 启动确认窗口 12s→90s：box64 冷启动 + UGC 模组下载常超 12s，
# 前端会误报"进程状态未确认"（实际启动仍在进行）
    && sed -i 's|d=async(m,v,b=12)|d=async(m,v,b=90)|g' /web/dist/assets/*.js
# 产物：/web/dist

# ---------------------------------------------------------------- builder ----
FROM golang:1.24 AS builder

# 先拷 go.mod/go.sum 单独下载依赖，充分利用层缓存
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

# 交叉编译纯 Go 二进制（glebarez/sqlite 无 cgo，交叉编译零障碍）
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /out/dst-admin-go cmd/server/main.go \
    && mkdir -p /out

# ---------------------------------------------------------------- runtime ----
FROM ubuntu:22.04

LABEL maintainer="dst-mac-server (fork of carrot-hu23/dst-admin-go)"
LABEL description="DST dedicated server panel for Apple Silicon (linux/arm64). Steamcmd replaced by DepotDownloader, game runs under box64."

ENV DEBIAN_FRONTEND=noninteractive

# ===== 基础依赖 =====
# screen: 面板开服用；procps: 面板进程管理依赖 ps；tzdata: 时区
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl wget screen unzip tzdata procps \
    && ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone \
    && rm -rf /var/lib/apt/lists/*

# ===== .NET 8 runtime（DepotDownloader 依赖，Microsoft 源）=====
RUN wget -qO /tmp/packages-microsoft-prod.deb \
      https://packages.microsoft.com/config/ubuntu/22.04/packages-microsoft-prod.deb \
    && dpkg -i /tmp/packages-microsoft-prod.deb \
    && apt-get update \
    && apt-get install -y --no-install-recommends dotnet-runtime-8.0 \
    && rm -rf /var/lib/apt/lists/* /tmp/packages-microsoft-prod.deb

# ===== amd64 运行库（构建期装齐，不放 entrypoint）=====
# box64 运行的 DST 服务端是 x86_64 ELF，需要 amd64 版 glibc/libstdc++/libgcc/libcurl-gnutls。
# arm64 Ubuntu 的源在 ports.ubuntu.com（无 amd64 包），必须把原生源限定为 arm64，
# 并单独写一份指向 archive.ubuntu.com 的 amd64 源，否则 apt 会到 ports 拉 amd64 索引报 404。
RUN dpkg --add-architecture amd64 \
    && sed -i 's|^deb |deb [arch=arm64] |' /etc/apt/sources.list \
    && printf '%s\n' \
      'deb [arch=amd64] http://archive.ubuntu.com/ubuntu/ jammy main universe restricted multiverse' \
      'deb [arch=amd64] http://archive.ubuntu.com/ubuntu/ jammy-updates main universe restricted multiverse' \
      'deb [arch=amd64] http://archive.ubuntu.com/ubuntu/ jammy-security main universe restricted multiverse' \
      > /etc/apt/sources.list.d/amd64.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
      libc6:amd64 libstdc++6:amd64 libgcc-s1:amd64 libcurl3-gnutls:amd64 \
    && rm -rf /var/lib/apt/lists/*

# ===== box64（x86_64 → ARM64 用户态转译，DST 服务端运行时依赖）=====
# python3：box64 的 CMake 构建脚本需要 Python >= 3.7 解释器
# 构建完即清理编译工具链，避免撑大最终镜像
RUN apt-get update && apt-get install -y --no-install-recommends \
      git cmake build-essential python3 \
    && git clone --depth 1 --branch v0.4.5-1 https://github.com/ptitSeb/box64.git /tmp/box64 \
    && cmake -S /tmp/box64 -B /tmp/box64/build \
      -DARM_DYNAREC=ON -DCMAKE_BUILD_TYPE=RelWithDebInfo \
    && cmake --build /tmp/box64/build -j"$(nproc)" \
    && cmake --install /tmp/box64/build \
    && rm -rf /tmp/box64 \
    && apt-get purge -y --auto-remove git cmake build-essential python3 \
    && rm -rf /var/lib/apt/lists/*

# ===== DepotDownloader（替代 steamcmd：游戏安装/更新与创意工坊模组下载）=====
# 面板代码默认路径为 /opt/DepotDownloader/DepotDownloader，勿改动目录结构
RUN wget -qO /tmp/DepotDownloader.zip \
      https://github.com/SteamRE/DepotDownloader/releases/download/DepotDownloader_3.4.0/DepotDownloader-linux-arm64.zip \
    && mkdir -p /opt/DepotDownloader \
    && unzip -o /tmp/DepotDownloader.zip -d /opt/DepotDownloader \
    && chmod +x /opt/DepotDownloader/DepotDownloader \
    && rm -f /tmp/DepotDownloader.zip

# ===== 面板本体与配置 =====
WORKDIR /app
COPY --from=builder /out/dst-admin-go /app/dst-admin-go
RUN chmod 755 /app/dst-admin-go

COPY --from=frontend /web/dist /app/dist
COPY static/ /app/static
COPY config.yml /app/config.yml

# 播种用默认集群配置：烤在稳定路径而非直接放 /app/data，
# 因为 /app/data 会被 volume 挂载，空卷会遮住镜像内文件；
# entrypoint 首启时仅在 /app/data/dst_config 缺失时才拷入。
COPY scripts/docker-arm64/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod 755 /app/docker-entrypoint.sh
COPY scripts/docker-arm64/docker_dst_config /app/docker_dst_config.default

# 游戏本体安装目录（entrypoint 条件首装；建议挂卷持久化避免重复下载 ~2GB）
ENV DST_DIR=/app/dst-dedicated-server
# 标记容器构建：面板据此锁定与镜像固定路径绑定的设置项（对齐 amd64 镜像行为）
ENV DST_ADMIN_CONTAINER_MODE=true

# 面板端口
EXPOSE 8082/tcp
# 饥荒世界通信端口
EXPOSE 10888/udp
# 洞穴世界端口
EXPOSE 10998/udp
# 森林世界端口
EXPOSE 10999/udp

ENTRYPOINT ["./docker-entrypoint.sh"]
