# rpf

[README](README.md) | [中文文档](README_zh.md)

`rpf` 是一个小型反向 TCP 端口转发工具，行为类似：

```text
ssh -R [bind_address:]port:host:hostport
```

它使用自定义的最小协议，不兼容 SSH 协议。v1 实现仅支持 TCP，
只使用 Go 标准库，并且每个客户端进程支持一个反向转发。

## 功能特性

- **类似 `ssh -R` 的反向 TCP 转发**：服务端绑定指定的远端地址，并将每个入站连接转发到客户端侧目标地址。
- **小型单文件部署**：只有 `server` 和 `client` 两个子命令。
- **纯 Go 标准库实现**：没有外部运行时服务、数据库或第三方包依赖。
- **可复用的多客户端服务端**：每个认证客户端拥有一个远端监听器；只要远端绑定地址不冲突，多个客户端可以共用同一个服务端。
- **客户端声明转发关系**：客户端发送 `--remote` 和 `--target`，服务端保持通用，不需要为每条隧道维护配置文件。
- **接近 OpenSSH 的远端绑定语义**：支持 `8080`、`127.0.0.1:8080`、`:8080`、`*:8080` 和带方括号的 IPv6 地址。
- **一个服务端端口承载控制连接和数据连接**：第一行协议头标识 `CONTROL` 或 `DATA`，合法的数据连接随后切换为原始 TCP 转发。
- **每个入站连接对应一个新的目标连接**：每个远端 TCP 连接都会映射到客户端侧的一条新目标连接。
- **面向重连的客户端行为**：绑定失败、权限错误和控制连接断开都会按固定可配置间隔重试，只要客户端进程仍在运行。
- **陈旧隧道清理**：服务端通过 `PING` / `PONG` 心跳检测半开或失效的控制连接，同时服务端和客户端都支持 SIGINT/SIGTERM 清理。
- **支持半关闭的双向转发**：保持正常 TCP EOF 语义，活跃转发流不设置空闲超时。
- **仅回环监听的状态接口**：`GET /status` 返回当前计数、累计计数和活跃隧道摘要，不包含 token 或连接 ID。
- **服务端资源控制**：包含 pending open 上限、活跃转发连接上限、初始协议头读取超时和 pending 数据连接等待超时。
- **不会记录密钥的 token 认证**：token 来自 `--token` 或 `RPORT_TOKEN`，使用常量时间比较，并且不会出现在状态响应或日志中。
- **明确的安全边界**：token 认证不是加密。如果流量经过不可信网络，请使用可信网络、VPN、TLS 包装或其他传输安全层。
- **有意收窄的 v1 范围**：仅 TCP、自定义协议、每个客户端进程一条隧道、不兼容 SSH、无内置 TLS、无 Unix socket、无状态接口认证、无绑定 ACL，不支持远端或目标端口 `0`。

## 构建

构建当前平台二进制：

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o rpf .
```

交叉编译 Linux AMD64：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o rpf-linux-amd64 .
```

## 使用

启动服务端：

```bash
RPORT_TOKEN=secret ./rpf server
```

启动客户端隧道：

```bash
RPORT_TOKEN=secret ./rpf client \
  --server 127.0.0.1:9000 \
  --remote 127.0.0.1:8080 \
  --target 127.0.0.1:3000
```

查看服务端状态：

```bash
curl http://127.0.0.1:9001/status
```

## 参数

服务端：

```bash
rpf server [--listen :9000] [--status-listen 127.0.0.1:9001] [--open-timeout 10s] [--max-pending 128] [--max-active 1024] [--heartbeat-interval 30s] [--heartbeat-timeout 90s] [--token secret]
```

客户端：

```bash
rpf client --server host:port --remote [bind_address:]port --target host:hostport [--token secret] [--reconnect-interval 5s]
```

`--token` 优先于 `RPORT_TOKEN`。token 必填，不能为空，也不能包含空白字符。

`--max-pending` 和 `--max-active` 是每条隧道的资源上限。当隧道达到容量上限时，额外远端调用方会被关闭，而不是无限排队。

`--heartbeat-interval` 和 `--heartbeat-timeout` 控制服务端发起的控制连接心跳。如果客户端没有在超时时间内用 `PONG` 响应 `PING`，服务端会关闭该隧道并释放远端监听器。

## 作为后台服务运行

创建服务文件 `/lib/systemd/system/reverse-port-forwarding.service`：

```conf
[Unit]
# describe the app
Description=reverse-port-forwarding
# start the app after the network is available
After=network.target

[Service]
# usually you'll use 'simple'
# one of https://www.freedesktop.org/software/systemd/man/systemd.service.html#Type=
Type=simple
# which user to use when starting the app
User=rpf
# path to your application's root directory
WorkingDirectory=/opt/reverse-port
# the command to start the app
# requires absolute paths
Environment="RPORT_TOKEN=change-this-token"
ExecStart=/opt/reverse-port/rpf server --listen :9000 --status-listen 127.0.0.1:9001
KillSignal=SIGTERM
# How long systemd waits for the app to stop before sending SIGKILL
TimeoutStopSec=30s
# Ensures systemd only considers the main process for the stop signal
KillMode=control-group
# restart policy
# one of {no|on-success|on-failure|on-abnormal|on-watchdog|on-abort|always}
Restart=always

[Install]
# start the app automatically
WantedBy=multi-user.target
```

## 工作方式

`rpf` 使用两类 TCP 连接：**控制连接** 和 **数据连接**。

```text
                         ┌─────── Server ───────┐         ┌─── Client ───┐
                         │                      │         │              │
  remote caller ────────▶│ :8080  (remote)      │  OPEN   │              │
                         │    │                 │────────▶│              │
                         │    ▼                 │         │   ┌──────┐   │
                         │  (pending)           │         │   │target│   │
                         │                      │  DATA   │   │:3000 │   │
                         │ :9000  (tunnel)      │◀────────│   └──▲───┘   │
                         │                      │         │      │       │
                         └──────────────────────┘         └──────┴───────┘
```

涉及三类 TCP 连接：

- **control**：客户端到服务端隧道端口的持久连接
- **data**：每个转发请求对应一条客户端回连服务端的数据连接
- **target**：客户端侧到本地服务的连接

1. **启动。** 客户端连接服务端隧道端口，发送包含 token、远端绑定地址和目标地址的 `CONTROL` 头。服务端回复 `OK`，并在远端地址启动 TCP 监听。
2. **转发。** 远端调用方连接远端监听器时，服务端暂存该连接，并通过控制通道向客户端发送 `OPEN <id>`。
3. **数据连接附着。** 客户端再次连接服务端，发送包含 token 和连接 ID 的 `DATA` 头，然后连接本地目标服务。数据连接建立后，服务端将远端调用方流量转发到数据连接，客户端再将数据连接转发到目标服务。
4. **重连。** 如果控制连接断开，客户端按固定间隔重试。会话之间不保留状态。

## 远端地址语义

- `--remote 8080` 绑定 `127.0.0.1:8080`
- `--remote 127.0.0.1:8080` 显式绑定回环地址
- `--remote :8080` 绑定所有接口
- `--remote '*:8080'` 绑定所有接口
- `--remote 0.0.0.0:8080` 绑定所有 IPv4 接口
- `--remote '[::1]:8080'` 使用带方括号的 IPv6 回环地址
- `--remote '[::]:8080'` 绑定所有 IPv6 接口

远端和目标端口 `0` 都会被拒绝。

## 安全说明

`rpf` 使用共享 token 认证控制连接和数据连接，但不提供加密。需要流量保密时，请在可信网络中使用，或使用 VPN/TLS 包装。

状态接口 v1 没有认证，并且有意限制为仅回环地址监听。状态响应不包含 token 或连接 ID。

## 资源占用

对于 N 个已连接客户端和 M 个同时活跃的转发连接：

| 资源 | 公式 |
|---|---|
| 已建立 TCP 连接 | N + 3M |
| 服务端监听器 | N + 2 |
| 总 goroutine 数量（服务端 + 所有客户端） | 2 + 4N + 8M |

每条隧道的开销：1 条持久 TCP 连接，4 个 goroutine（服务端 2 个，客户端 2 个）。每条活跃转发连接的开销：3 条 TCP 连接，8 个 goroutine（服务端 4 个，客户端 4 个）。

## 验证

```bash
go test ./...
```
