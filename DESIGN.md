# cusconf — 设计草案

## 目标
从机场订阅 / 自建节点得到节点 → 可视化配置策略组、规则、规则集源、图标 → 按「客户端 + 版本」生成配置 → 通过带时效的 URL 让终端直接导入。

## 核心数据流

```
Source(订阅URL / 手动节点 / 本地文件)
   │  fetch(带UA、缓存、失败回退)
   ▼
Parser(多格式解析: base64 URI 列表 / Clash YAML / Surge conf / sing-box JSON / QX / Loon …)
   ▼
Node IR(统一节点模型: type, server, port, tls, transport, 协议专属字段)
   │  过滤 / 重命名 / 去重 / 打标签(地区、倍率、测试结果)
   ▼
Profile(用户配置: 策略组、规则、规则集、DNS、通用项、目标客户端+版本)
   ▼
Renderer(Surge / Stash … 每个 renderer 查询 Capability Matrix)
   ▼
Output(带 token + 过期时间的订阅 URL;可短链)
```

## 关键模块

| 模块 | 说明 |
|---|---|
| **Parser** | 参考 Sub-Store 的解析器思路,TS 重写,每种格式一个 parser,产出 Node IR;单元测试用真实样例 |
| **Node IR** | 协议: ss / ssr / vmess / vless(reality, xtls-flow) / trojan / hysteria2 / tuic / wireguard / snell / anytls / http / socks5 … |
| **Capability Matrix** | `client × version → 支持的协议、参数、策略组类型、规则类型`。例: Surge 某版本起支持 vless、`smart` 组;低版本自动降级(smart→url-test)或剔除不支持节点,并在生成结果里给出警告列表 |
| **Renderer** | Surge(.conf)、Clash 系 YAML(Stash 与 mihomo 两种方言,mihomo 供 Clash Meta / FlClash)、sing-box(JSON)、Loon、Quantumult X;节点参数映射按 matrix 处理(如 Surge 的 vless 字段写法与 Clash 不同) |
| **Rule-set Mirror** | `/rs/{id}.{list,txt,yaml,json,qx}`:生成配置时登记用到的规则集地址(只镜像管理员配置的地址),服务端下载缓存(SQLite)并按客户端转换格式;sing-box / QX 依赖它做格式转换 |
| **Short Link** | 订阅 token 可生成 6 位短码 `/s/{code}`,与 `/sub/{token}` 等价,配置里写入的更新地址随访问方式而定 |
| **Policy Group 编辑器** | 可视化建组:手动选节点 / 按正则·地区·标签·测试结果筛选;组类型按目标客户端可选(select, url-test, fallback, load-balance, smart…);图标库 |
| **Rules / Rule-sets** | 预设规则集(源可配置,如 blackmatrix7、Loyalsoldier、MetaCubeX),自定义规则,规则集源镜像/代理下载 |
| **Node Tester** | 以独立进程运行 mihomo(GPL-3.0,不链接进 cusconf):每次测试生成一份配置,每个节点一个绑定该节点的 SOCKS5 listener,cusconf 经由它们请求;mihomo 拒绝的节点用 `mihomo -t` 逐个剔除:<br>• 出口 IP + 风险评分(可自定义评分源: HTTP 模板 + JSONPath 提取)<br>• 流媒体/服务可用性(Netflix、Disney+、YouTube Premium、ChatGPT、TikTok…)与地区<br>• 延迟/丢包<br>结果写回节点标签,供策略组筛选 |
| **Admin / Deploy** | Web 后台;Docker 单容器(SQLite);订阅 URL token 可设过期、访问次数、吊销;定时刷新订阅与测试 |

## 已定
- **技术栈**: Go 后端(chi + SQLite/`modernc.org/sqlite`,纯 Go 无 CGO)+ React(Vite + TS + shadcn/ui),前端构建产物 `embed` 进 Go 二进制,单 Docker 镜像。节点测试以独立进程方式运行 mihomo 可执行文件(许可证隔离:cusconf 为 Apache-2.0 + Commons Clause,mihomo 为 GPL-3.0)。
- **用户模型**: 单管理员;可为每个 Profile 生成多个订阅 token(过期时间、吊销、访问日志)。
- **M1 范围**: 主链路,节点测试放 M2。

## 目录结构
```
cmd/cusconf/          入口
internal/node/        Node IR
internal/parser/      各订阅格式解析器(+ testdata 样例)
internal/source/      订阅拉取、缓存、定时刷新
internal/capability/  client×version 能力矩阵(YAML 数据驱动)
internal/render/      surge/ clash/ singbox/ loon/ qx/
internal/ruleset/     规则集解析、格式转换与镜像
internal/profile/     策略组、规则、规则集模型
internal/store/       SQLite
internal/api/         REST + 订阅输出 /sub/{token}
web/                  React 后台
```

## 里程碑
**M1 主链路** — ✅ 已完成（2026-11-21）
1. Node IR + 解析器: URI 列表(ss/ssr/vmess/vless/trojan/hy2/tuic/wg/anytls)、base64、Clash/mihomo YAML、Surge proxy 行;黄金样例测试
2. 能力矩阵 + Surge / Stash renderer,按版本降级并输出告警
3. Profile 模型:策略组(手动 / 正则 / 地区筛选)、规则、规则集源、图标
4. 存储 + REST API + `/sub/{token}`(过期、吊销)
5. React 后台:订阅源、节点列表、策略组可视化编辑、规则集、生成预览、URL 管理
6. Dockerfile + compose

**M2 节点测试** — ✅ 首版已完成（2026-11-22）: 内嵌 mihomo;出口 IP + 自定义风险评分源;流媒体/服务解锁与地区;结果作为节点标签参与策略组筛选;定时任务。

**M3 更多客户端、短链、规则集镜像** — 首版（2026-11-22）
1. Clash Meta / FlClash:与 Stash 共用 Clash 渲染器,mihomo 方言(hysteria2 password/up/down、fake-ip DNS、rule-provider path 等);输出经 mihomo 解析器校验
2. sing-box:1.11+ 格式(rule action、WireGuard endpoint),1.12+ 新 DNS 服务器格式;GEOIP/GEOSITE 转为 SagerNet 的远程 .srs
3. Loon、Quantumult X:INI 风格渲染器;远程规则集走 [Remote Rule] / [filter_remote]
4. 规则集镜像(见上表)与短链接
5. 待办:Loon / QX / FlClash 的能力矩阵版本号核对(`verified: false`);sing-box 输出在 CI 中用 `sing-box check` 校验
