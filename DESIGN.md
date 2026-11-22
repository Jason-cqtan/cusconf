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
| **Renderer** | Surge(.conf)、Stash(YAML,Clash 系扩展);节点参数映射按 matrix 处理(如 Surge 的 vless 字段写法与 Clash 不同) |
| **Policy Group 编辑器** | 可视化建组:手动选节点 / 按正则·地区·标签·测试结果筛选;组类型按目标客户端可选(select, url-test, fallback, load-balance, smart…);图标库 |
| **Rules / Rule-sets** | 预设规则集(源可配置,如 blackmatrix7、Loyalsoldier、MetaCubeX),自定义规则,规则集源镜像/代理下载 |
| **Node Tester** | 后台内置 mihomo 内核作为测试代理:为每个节点开临时 listener,通过它请求:<br>• 出口 IP + 风险评分(可自定义评分源: HTTP 模板 + JSONPath 提取)<br>• 流媒体/服务可用性(Netflix、Disney+、YouTube Premium、ChatGPT、TikTok…)与地区<br>• 延迟/丢包<br>结果写回节点标签,供策略组筛选 |
| **Admin / Deploy** | Web 后台;Docker 单容器(SQLite);订阅 URL token 可设过期、访问次数、吊销;定时刷新订阅与测试 |

## 已定
- **技术栈**: Go 后端(chi + SQLite/`modernc.org/sqlite`,纯 Go 无 CGO)+ React(Vite + TS + shadcn/ui),前端构建产物 `embed` 进 Go 二进制,单 Docker 镜像。第二阶段测试直接以库方式引入 mihomo。
- **用户模型**: 单管理员;可为每个 Profile 生成多个订阅 token(过期时间、吊销、访问日志)。
- **M1 范围**: 主链路,节点测试放 M2。

## 目录结构
```
cmd/cusconf/          入口
internal/node/        Node IR
internal/parser/      各订阅格式解析器(+ testdata 样例)
internal/source/      订阅拉取、缓存、定时刷新
internal/capability/  client×version 能力矩阵(YAML 数据驱动)
internal/render/      surge/ stash/
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

**M2 节点测试** — 进行中（2026-11-21 完成首版）: 内嵌 mihomo;出口 IP + 自定义风险评分源;流媒体/服务解锁与地区;结果作为节点标签参与策略组筛选;定时任务。

**M3**: 更多客户端(Clash Meta、sing-box、Loon、QX)、短链、规则集镜像。
