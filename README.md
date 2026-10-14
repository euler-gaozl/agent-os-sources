# Agent OS Sources

独立的公开资料采集服务与 MCP 适配器，拥有 `sources` schema 的原始内容和采集元数据。先留存响应，再返回提取内容、URL、采集时间与 SHA-256；采集时间不等于发布日期或统计期。服务不推导事实、不预测价格、不决定交易。

## 采集能力与限制

提供运维批准的来源目录、指定 URL 采集、不可变正文分页，以及留存页面/链接内的字面查找。目前不是通用网络搜索引擎，不执行浏览器 JavaScript，没有 PDF/OCR，也不绕过登录或付费限制。

只访问 HTTPS 白名单主机，限制 IP/内网目标、嵌入凭证、敏感查询参数和重定向。`SOURCE_ALLOWED_HOSTS` 只能由运维配置。响应限制为原始 1 MiB、解码后 3 MiB，支持文本、HTML、JSON、XML；原始正文指经过 HTTP 传输/内容解码的响应体，不是网络抓包字节。保存资料不代表已证明其真实性。

## 管理采集与知识库导入

中台调用 `POST /captures` 前生成 32 位小写十六进制 ID。同 ID、同精确 URL 返回原观察，不同 URL 拒绝；丢回复后通过 `GET /captures/{id}` 回查。首次并发请求可能多次只读访问网站，但只有一份完整观察提交，因此不承诺网络请求恰好一次。

`GET /captures?url=<精确URL>&after=<采集ID>` 每页最多 20 条。Knowledge 身份可读 `/capture-capabilities` 和 `/captures/{id}/bundle`，不能发起采集或任意列举历史。导入包保留正文、原始响应和哈希，单项超过 1 MiB 拒绝导入，不截断 Sources 中已存资料。

## 构建、迁移与部署

需要 Go 1.27.2、PostgreSQL 18；`scripts/Build.ps1` 需要 PowerShell 7。也可运行 `go build -mod=vendor ./cmd/sources`。设置 `GOWORK=off`、`GOPROXY=off`，使用固定 SDK 与提交的 `vendor`，无需相邻仓库、Node 或模型 Key。

按 [配置示例](deploy/service.example.json) 提供 `search_path=sources` 的非所有者应用账号、独立服务身份/对端哈希（含自身），内部端口使用私网/TLS。MCP 凭证由 Registry 按名称解析，中台可管理下载，其他身份不能绕过鉴权。

使用独立 DDL 连接 `MIGRATION_DATABASE_URL` 执行 `bin/migrate --role <账号>`。迁移只修改本 schema 和授权；采集管理版本增加 `extraction_version`，旧记录保留 `unrecorded`。服务 `bin/sources --config <文件>` 启动只做就绪检查，不做 DDL。HTTP 并发默认 4、连接池默认 3。

Windows 维护器可使用 `cmd/migrate --config <受保护JSON>`，该文件最多 64 KiB，仅允许 `MIGRATION_DATABASE_URL` 与 `ROLE`，不能和环境 DDL 连接或 `--role` 混用。连接串不进入命令行，错误不回显配置；客户端退出不能单独证明 DDL 已提交，恢复写入前必须检查实际数据库。

按 [发布契约](deploy/release-contract.json) 独立升级/回滚，保留原始采集、数据库和配置，不自动删除采集。容器/多机显式设置服务地址。

## 验证状态

运行 `go test -mod=vendor -count=1 ./...` 与 `go vet -mod=vendor ./...`。`TEST_DATABASE_URL` 启用独立测试库，覆盖权限、迁移回滚、原始字节、不可变分页、超限拒绝和中文 UTF-8；没有数据库时明确跳过。合成 HTTP 不证明真实网站可用或商品覆盖。

Windows 正式采集/入库/恢复见 [中台采集验收](https://github.com/euler-gaozl/agent-os/blob/main/docs/console-capture-acceptance-20261014.md)。Actions 日常关闭；[历史细节](docs/history/service-notes-20261014.en.md) 保留具体契约与记录。
