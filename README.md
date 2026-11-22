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
2. **配置**：新建配置（自带常用策略组与 ACL4SSR 规则集），编辑策略组、规则，在「生成预览」里切换客户端与版本查看结果和兼容性提示。
3. **订阅链接**：选择配置 + 客户端 + 版本 + 有效期，复制链接或一键导入 Surge / Stash。

## 节点测试

「节点」页可以对当前筛选出的节点做测试：

- **测延迟**：通过内置的 mihomo 内核连接节点，按 url-test 的方式测一次往返。
- **完整测试**：除延迟外，还会检测：
  - 出口 IP、地区与安全分（0–100，越大越安全）；
  - 服务是否可用：Netflix、Disney+、YouTube Premium、HBO Max、Prime Video、Hulu、Spotify、TikTok、ChatGPT、Claude、Gemini、Steam、PayPal、任天堂。

点击结果可以查看各数据源和各服务的详情；鼠标移到某一行，末尾的 ▶ 可以单独立即测试这个节点。机场信息节点不测试，工具栏可以只选部分订阅源测试。右上角齿轮里可以设置：

- 并发数、超时；
- 定时完整测试；
- 要检测的服务；
- 出口 IP / 评分源：默认有 ip-api.com（地区、代理/机房标记，无评分）、proxycheck.io、IPPure、Net.Coffee 四个，各来源的安全分分别显示、不合并；也可以添加任意 JSON API，用 JSON 路径映射字段；地址中的 `{ip}` 会代入出口 IP。分数按各网站自身的含义换算成安全分（越大越安全）：IPPure、proxycheck.io 是风险分，取 100 − 风险分；Net.Coffee 是信任分，直接使用；自定义源按风险分处理。默认的 proxycheck.io（v3 接口，和其网站显示一致）不带 Key 时每个服务器 IP 每天只能查 100 次，节点多时请到 proxycheck.io 免费注册，并在地址后加 `?key=你的Key`（每天 1000 次）。

测试结果可以在策略组的「按测试结果筛选」里使用，例如只要可用、延迟低于 300 ms、某个来源的安全分不低于 50、出口在美国、Netflix 可用的节点。未测试的节点不满足这些条件。

服务检测基于公开页面的特征判断，网站改版时可能失准。Claude、Gemini、Hulu 无法直接探测，按出口地区推断；Steam 显示商店货币，任天堂显示官网所属区域。

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
