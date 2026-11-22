# cusconf

把机场订阅和自建节点，按可视化配置的策略组与规则，生成适配客户端版本的 Surge / Stash / Clash Meta / FlClash / sing-box / Loon / Quantumult X 配置，并通过带时效的订阅链接导入。

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
3. **订阅链接**：选择配置 + 客户端 + 版本 + 有效期，复制链接或一键导入（Surge / Stash / Clash Meta / FlClash / sing-box）。可以为链接生成短链接（`/s/xxxxxx`），方便在手机上手动输入；完整链接仍然可用，删除短链接后用短链接导入的客户端将无法更新。

界面支持中文 / English 和白天 / 夜晚 / 跟随系统三种外观，在左下角切换，设置保存在当前浏览器。英文翻译在 [web/src/en.ts](web/src/en.ts)，以中文原文为键；漏译的文字会显示中文。

## 规则集镜像

规则集的原地址（通常在 GitHub）可以由本服务代为下载、缓存并转换格式，客户端从 `/rs/…` 加载：

- **sing-box、Quantumult X 总是使用镜像**：它们的规则集格式与 Surge / Clash 不同，镜像会把 Surge / Clash 列表转换为 sing-box 源格式（JSON）或 Quantumult X 分流格式。sing-box 也可以在规则集里单独填写 `.srs` 地址，此时直接使用该地址。
- 在配置的「规则」页开启「所有客户端都通过本服务镜像加载规则集」后，其它客户端也改用镜像，无需直接访问 GitHub。
- 副本超过 12 小时会在客户端请求时重新下载，最近 7 天用过的列表也会在后台定时更新；原地址失败时继续提供上次的副本。只有生成配置时用到的地址才会被镜像。
- 客户端需要能访问本服务的公开地址（见 `CUSCONF_BASE_URL`）。

## 节点测试

「节点」页可以对当前筛选出的节点做测试：

- **测延迟**：通过 mihomo 内核连接节点，按 url-test 的方式测一次往返。
- **完整测试**：除延迟外，还会检测：
  - 出口 IP、地区与安全分（0–100，越大越安全）；
  - 服务是否可用：Netflix、Disney+、YouTube Premium、HBO Max、Prime Video、Hulu、Spotify、TikTok、ChatGPT、Claude、Gemini、Steam、PayPal、任天堂。

点击结果可以查看各数据源和各服务的详情；鼠标移到某一行，末尾的 ▶ 可以单独立即测试这个节点。机场信息节点不测试，工具栏可以只选部分订阅源测试。右上角齿轮里可以设置：

- 并发数、超时；
- 定时完整测试；
- 要检测的服务；
- 出口 IP / 评分源：默认有 ip-api.com（地区、代理/机房标记，无评分）、proxycheck.io、IPPure、Net.Coffee 四个，各来源的安全分分别显示、不合并；也可以添加任意 JSON API，用 JSON 路径映射字段；地址中的 `{ip}` 会代入出口 IP。所有来源都通过被测节点访问，不依赖本机的 DNS 和代理设置。分数按各网站自身的含义换算成安全分（越大越安全）：IPPure、proxycheck.io 是风险分，取 100 − 风险分；Net.Coffee 是信任分，直接使用；自定义源按风险分处理。默认的 proxycheck.io（v3 接口，和其网站显示一致）不带 Key 时每个发起查询的 IP（即各节点的出口 IP）每天只能查 100 次，多个节点共用出口时请到 proxycheck.io 免费注册，并在地址后加 `?key=你的Key`（每天 1000 次）。

测试结果可以在策略组的「按测试结果筛选」里使用，例如只要可用、延迟低于 300 ms、某个来源的安全分不低于 50、出口在美国、Netflix 可用的节点。未测试的节点不满足这些条件。

服务检测基于公开页面的特征判断，网站改版时可能失准。Claude、Gemini、Hulu 无法直接探测，按出口地区推断；Steam 显示商店货币，任天堂显示官网所属区域。

### 同步测试结果

有些节点（常见于 IEPL 等中转入口）只接受中国大陆网络的连接，部署在境外 VPS 上的 cusconf 测不通它们。可以在能连上的地方（如家里电脑上的 cusconf）测试，再把结果推送到服务器：

1. 在服务器的「测试设置 → 同步测试结果」里复制**本机的同步密钥**；
2. 在本地的同一位置填写服务器地址（如 `https://sub.example.com`）和这个密钥，点「保存并立即推送」；打开「每次测试完成后自动推送」后，每次测试完成都会推送；
3. 服务器按节点（服务器、端口、密码相同即同一节点）保存较新的结果，策略组的测试条件照常生效；服务器自己测试失败时不会覆盖同步来的可用结果，本机测通则以本机结果为准。

「同时推送订阅源的节点」默认开启：订阅地址只允许国内 IP 拉取时（服务器拉取返回 403 等），服务器也能有节点。服务器按订阅地址匹配同一个源（参数不同时按域名和路径，再按节点重合或名称），找不到就新建；服务器自己拉取成功且更新时，以服务器的为准，拉取失败时保留推送来的节点。

推送请求由本地 cusconf 直接发出；若本机开着 Surge 等代理软件，要确保它能解析并连上服务器地址。

## 环境变量

| 变量 | 说明 |
|---|---|
| `CUSCONF_ADMIN_PASSWORD` | 管理员密码。不设置时首次启动随机生成并打印在日志中 |
| `CUSCONF_BASE_URL` | 对外访问地址，用于 Surge `#!MANAGED-CONFIG` 和规则集镜像地址；不设置则按请求的 Host 推断 |
| `CUSCONF_ADDR` | 监听地址，默认 `:8080` |
| `CUSCONF_DATA` | 数据目录（SQLite），默认 `./data` |
| `CUSCONF_MIHOMO` | mihomo 可执行文件路径，用于节点测试。不设置时依次查找 cusconf 同目录下的 `mihomo` 和 `PATH`；Docker 镜像已自带 |

## 开发

本地运行节点测试需要 mihomo 可执行文件：从 [MetaCubeX/mihomo Releases](https://github.com/MetaCubeX/mihomo/releases) 下载对应平台的版本放进 `PATH`，或用 `CUSCONF_MIHOMO` 指定路径。没有 mihomo 时其它功能正常，只是节点测试无法运行。

```bash
make dev                 # 后端 :8080，密码 admin
cd web && npm run dev    # 前端热更新，代理 /api 到后端
make test
```

## 支持范围

- **解析**：ss（SIP002 / SS2022 / 旧格式 / obfs / shadow-tls）、ssr、vmess、vless（REALITY / Vision / ws / grpc / xhttp）、trojan、hysteria2、hysteria、tuic v4/v5、wireguard、anytls、socks5、http；容器格式支持 URI 列表、Base64、Clash/mihomo YAML、Surge、sing-box JSON。
- **生成**：Surge（iOS / Mac）、Stash（iOS / macOS）、Clash Meta（按 mihomo 内核版本）、FlClash、sing-box（1.11+；1.12 起使用新 DNS 格式）、Loon、Quantumult X。
  - 不同客户端的差异会给出提示，例如 sing-box 没有 fallback / load-balance 组（降级为 urltest）、策略组不能包含 REJECT；Loon 和 Quantumult X 的远程规则集与本地规则分开匹配，规则集之后的本地规则顺序会变化。
  - Clash Meta 的输出用 mihomo 自带的解析器做了校验（`go test ./internal/engine -run Mihomo`）；sing-box 可设置 `SINGBOX_BIN` 后运行 `go test ./internal/engine -run SingBoxCheck` 用 `sing-box check` 校验。
- **版本兼容**：[`internal/capability/matrix.yaml`](internal/capability/matrix.yaml) 记录每项特性的最低版本。低版本会自动处理不兼容的部分并输出提示：
  - 策略组类型降级，例如 `smart` → `url-test`
  - 去掉不支持的参数，例如 `icon-url`、`alpn`
  - 剔除不支持的节点，例如低版本 Surge 中的 VLESS
- 矩阵中标记 `verified: false` 的条目还没有和官方文档核对过；Clash Meta / FlClash / sing-box / Loon / Quantumult X 的不少版本号属于这一类。

## 许可证

cusconf 采用 **Apache License 2.0 + Commons Clause v1.0**（见 [LICENSE](LICENSE)），属于「源码可用、禁止售卖」，不是 OSI 定义的开源许可证。以下为通俗说明，以 LICENSE 原文为准：

- ✅ 可以免费使用、复制、修改、分发，可以在个人或公司内部部署。
- ❌ 不得售卖本项目；也不得售卖基于本项目修改、重命名、换壳、打包或以其它方式派生的版本。
- ❌ 不得以本项目（或其派生版本）为核心商品提供付费源码、付费安装包、付费 SaaS / 托管服务，或与之相关的付费咨询 / 支持服务。
- 再分发时须保留 LICENSE 与 NOTICE（包含 Commons Clause 声明）。

### 第三方组件

节点测试使用 [mihomo](https://github.com/MetaCubeX/mihomo)（GPL-3.0）。mihomo 不是 cusconf 的一部分，也没有链接进 cusconf 程序：cusconf 以独立进程的方式运行未经修改的 mihomo 可执行文件。Docker 镜像随附该可执行文件及其许可证（`/usr/share/licenses/mihomo/LICENSE`），源码见其仓库。mihomo 本身仍按 GPL-3.0 授权，上述禁止售卖的限制不适用于它。

