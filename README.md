# cusconf

把机场订阅和自建节点，按可视化配置的策略组与规则，生成适配客户端版本的 Surge / Stash 配置，并通过带时效的订阅链接导入。

## 快速开始

**Apple container**（macOS）：

```bash
make image
CUSCONF_ADMIN_PASSWORD=change-me make run
```

数据保存在项目下的 `./data`。`make logs` 查看日志，`make stop` 停止。Apple container 不支持 compose，也不会自动重启，Mac 重启后需要再执行 `make run`。

**Docker**：

```bash
CUSCONF_ADMIN_PASSWORD=change-me docker compose up -d --build
```

打开 http://localhost:8080 ，用管理员密码登录：

1. **订阅源**：添加机场订阅链接，或粘贴自建节点（分享链接 / Clash proxies / Surge [Proxy]）。
2. **配置**：新建配置（自带常用策略组与 blackmatrix7 规则集），编辑策略组、规则，在「生成预览」里切换客户端与版本查看结果和兼容性提示。
3. **订阅链接**：选择配置 + 客户端 + 版本 + 有效期，复制链接或一键导入 Surge / Stash。

## 环境变量

| 变量 | 说明 |
|---|---|
| `CUSCONF_ADMIN_PASSWORD` | 管理员密码。不设置时首次启动随机生成并打印在日志中 |
| `CUSCONF_BASE_URL` | 对外访问地址，用于 Surge `#!MANAGED-CONFIG`；不设置则按请求的 Host 推断 |
| `CUSCONF_ADDR` | 监听地址，默认 `:8080` |
| `CUSCONF_DATA` | 数据目录（SQLite），默认 `./data` |

## 开发

```bash
make dev                 # 后端 :8080，密码 admin
cd web && npm run dev    # 前端热更新，代理 /api 到后端
make test
```

## 支持范围

- **解析**：ss（SIP002 / SS2022 / 旧格式 / obfs / shadow-tls）、ssr、vmess、vless（REALITY / Vision / ws / grpc / xhttp）、trojan、hysteria2、hysteria、tuic v4/v5、wireguard、anytls、socks5、http；容器格式支持 URI 列表、Base64、Clash/mihomo YAML、Surge、sing-box JSON。
- **生成**：Surge（iOS / Mac）、Stash（iOS / macOS）。
- **版本兼容**：[`internal/capability/matrix.yaml`](internal/capability/matrix.yaml) 记录每项特性的最低版本。低版本会自动处理不兼容的部分并输出提示：
  - 策略组类型降级，例如 `smart` → `url-test`
  - 去掉不支持的参数，例如 `icon-url`、`alpn`
  - 剔除不支持的节点，例如低版本 Surge 中的 VLESS
- 矩阵中标记 `verified: false` 的条目还没有和官方文档核对过。
