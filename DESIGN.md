# Ops 运维平台（Go + Vue3）

## 当前落地架构

- Go 后端入口：`server/main.go` 使用 `tree.Spawn` 启动 HTTP Actor 和 Host Actor。
- HTTP Actor：`server/ops/internal/ops.go`，Gin 承载 HTTP、JWT 和基础 RBAC。
- 公共入口：`server/ops/external.go` 的 `ops.NewPos` 暴露 HTTP Actor。
- 主机 Actor：`server/ops/host/actor.go`，处理主机配置和 Docker 连通性测试。
- Docker 管理：`server/ops/host/docker.go`，统一使用 Docker TCP 2376 + TLS Client 连接。
- 持久化：当前用户、Token 和主机支持内存仓储；配置 `OPS_MONGO_URI` 后使用 MongoDB `ops_platform` 数据库。
- 前端：Vue3 + TypeScript + Vite + Pinia + Vue Router + Element Plus，独立由 nginx 部署。

## 运行后端

```powershell
$env:OPS_JWT_SECRET = "change-this-in-production"
$env:OPS_ADMIN_PASSWORD = "change-admin-password"
# 可选：启用 MongoDB
$env:OPS_MONGO_URI = "mongodb://127.0.0.1:27017"
cd server
go run .
```

健康检查：`GET http://127.0.0.1:8090/health`

登录：`POST /api/auth/login`

```json
{"username":"admin","password":"change-admin-password"}
```

## 当前目录约定

```text
server/
├── main.go
├── conf/
│   └── config.go
└── ops/
    ├── external.go       公共 Actor 入口
    ├── host/
    │   ├── actor.go       Host Actor
    │   ├── docker.go      Docker SDK + TCP TLS 2376 连接管理
    │   └── repository.go  主机配置仓储
    └── internal/
        ├── ops.go         Gin HTTP Actor
        ├── auth/
        ├── model/
        └── store/
```

后续新增 Actor 时直接在 `server/ops` 下新增目录，例如：

```text
server/ops/
├── deploy/
│   └── actor.go
├── service/
│   └── actor.go
└── audit/
    └── actor.go
```

HTTP Actor 通过 `tree.Request(...).AwaitTimeout(...)` 请求 Host Actor，避免把主机业务逻辑写入 Gin Handler。部署、拉镜像等长耗时操作后续应独立为 Operation/Deploy Actor。

## 主机 API

```text
GET    /api/hosts
POST   /api/hosts
DELETE /api/hosts/:id
POST   /api/hosts/:id/test
```

所有主机统一使用 Docker Engine TCP 2376 + TLS。证书路径指向运行 Go 后端的服务器文件，不能指向浏览器客户端路径。

配置示例：

```json
{
  "name": "game-1",
  "docker_host": "tcp://192.168.1.100:2376",
  "tls_ca": "/etc/ops/tls/ca.pem",
  "tls_cert": "/etc/ops/tls/client-cert.pem",
  "tls_key": "/etc/ops/tls/client-key.pem",
  "note": "游戏服 Docker 主机"
}
```

DockerManager 只使用：

```go
client.NewClientWithOpts(
    client.WithHost(host.DockerHost),
    client.WithTLSClientConfig(host.TLSCA, host.TLSCert, host.TLSKey),
    client.WithAPIVersionNegotiation(),
)
```

不再支持 SSH、SSH 私钥、SSH 密码、本地 Unix Socket 或 Docker 2375 非 TLS 连接。

## 服务类型管理

服务类型由 `server/ops/service` Actor 管理，支持结构化参数：

```json
{
  "host_id": "<host-id>",
  "name": "game",
  "image": "gogs-game:v1",
  "params": [
    {"flag": "--server-id", "value": "{{id}}"},
    {"flag": "--etcd", "value": "{{etcd}}"}
  ]
}
```

API：

```text
GET    /api/service-types
POST   /api/service-types
PUT    /api/service-types/:id
DELETE /api/service-types/:id
```

前端页面：`frontend/src/views/Services.vue`，支持动态增删参数行、镜像配置、参数预览和编辑删除。写操作仅 admin 可用。
