# syl-listing

`syl-listing` 是一个基于 Go + Cobra 的 CLI，用于把 Listing 需求 Markdown 批量生成中英 Listing Markdown 文件。

参考文档与样例：

- 需求模板：`doc/listing 模板-v1.md`
- 规则参考：`doc/亚马逊Listing生成指南（中英文版）.md`

## 安装

### macOS（Homebrew）

安装（首次/已 tap 过都可用）：

```bash
brew update && brew install hooziwang/tap/syl-listing
```

升级：

```bash
brew update && brew upgrade hooziwang/tap/syl-listing
```

如果提示 `No available formula`（本地 tap 索引过期）：

```bash
brew untap hooziwang/tap && brew install hooziwang/tap/syl-listing
```

### Windows（Scoop）

安装：

```powershell
scoop update; scoop bucket add hooziwang https://github.com/hooziwang/scoop-bucket.git; scoop install syl-listing
```

升级：

```powershell
scoop update; scoop update syl-listing
```

如果提示找不到应用（bucket 索引过期）：

```powershell
scoop bucket rm hooziwang; scoop bucket add hooziwang https://github.com/hooziwang/scoop-bucket.git; scoop update; scoop install syl-listing
```

## 快速开始（1 分钟）

1. 准备需求文件（首行必须是 `===Listing Requirements===`）：

```md
===Listing Requirements===
品牌名: DemoBrand
分类: Home & Kitchen
关键词库:
- keyword one
- keyword two
```

2. 首次运行（会自动初始化 `~/.syl-listing/config.yaml`、`~/.syl-listing/rules/`、`~/.syl-listing/.env.example`）：

```bash
syl-listing demo.md
```

3. 填写密钥：

```bash
cp ~/.syl-listing/.env.example ~/.syl-listing/.env
# 编辑 ~/.syl-listing/.env，填入可用 key
```

4. 再次运行生成：

```bash
syl-listing demo.md
```

5. 输出文件：

- `listing_xxxxxxxx_en.md`
- `listing_xxxxxxxx_cn.md`

## 命令

```bash
syl-listing [file_or_dir ...]
syl-listing gen [file_or_dir ...]
syl-listing version
```

常用示例：

```bash
# 单文件
syl-listing pinpai.md

# 多文件 + 每个文件生成 3 份候选 + 指定输出目录
syl-listing a.md b.md -n 3 -o ./out

# 目录输入 + 详细 NDJSON + 日志落盘
syl-listing ./requirements --verbose --log-file ./run.ndjson

# 使用子命令形式
syl-listing gen ./requirements -n 2
```

## 自动初始化

首次运行会自动创建：

- `~/.syl-listing/config.yaml`
- `~/.syl-listing/rules/`（仅目录，不生成默认规则文件）
- `~/.syl-listing/rules.lock`（规则中心同步状态）
- `~/.syl-listing/.env.example`

并要求手动创建：

- `~/.syl-listing/.env`

## .env 必填项

实际读取变量名由 `api_key_env` 控制，默认只需要：

```dotenv
# DeepSeek（英文生成 + 中文翻译都使用该 key）
DEEPSEEK_API_KEY=
```

## 规则文件（集中管理）

规则目录默认：`~/.syl-listing/rules`

- `title.yaml`
- `bullets.yaml`
- `description.yaml`
- `search_terms.yaml`

每一步只加载对应规则文件。每个规则文件同时包含：

- `instruction`：给模型的规则描述
- `constraints` / `output`：给程序校验的结构化约束

程序直接把该规则文件原文作为 `system`，并解析同一文件做校验，不做二次拼接。
规则推荐由独立仓库 `syl-listing-rules` 统一发布，客户端启动时自动同步到本地缓存目录 `~/.syl-listing/rules`。

不建议终端用户手改本地规则文件；规则变更应通过规则中心仓库发版。

`title.yaml` 最小示例：

```yaml
version: 1
section: title
language: en
purpose: 生成英文标题
output:
  format: text
  lines: 1
constraints:
  max_chars:
    value: 200
    hard: true
instruction: |
  只输出 1 行英文标题，不要解释。
```

`bullets.yaml`、`description.yaml`、`search_terms.yaml` 与此结构一致，仅 `section`、`output`、`constraints` 不同。

## 生成流程

1. EN 模型分段生成：`title -> bullets -> description -> search_terms`
2. CN 按 EN 分段翻译得到（标题/关键词/分类/五点/描述/搜索词）
3. 两个版本分别渲染输出

## 输入识别

- 输入可为单文件、多文件或目录。
- 目录递归扫描，仅处理 `.md`。
- 需求文件首个非空行必须是：

```text
===Listing Requirements===
```

## 输出

每个候选生成 2 个文件：

- `listing_xxxxxxxx_en.md`
- `listing_xxxxxxxx_cn.md`

`xxxxxxxx` 为 8 位随机串（数字 + 大小写字母），冲突自动重试。

## 日志输出

- 默认：终端输出简洁的人类可读进度日志。
- `--verbose`：终端输出详细 NDJSON（机器友好，包含模型请求/响应内容）。
- `--log-file`：额外写入 NDJSON 到文件；默认模式下仅文件是 NDJSON。

## 配置文件示例

```yaml
provider: deepseek
api_key_env: DEEPSEEK_API_KEY
rules_dir: ~/.syl-listing/rules
rules_center:
  owner: hooziwang
  repo: syl-listing-rules
  release: latest
  asset: rules-bundle.tar.gz
  timeout_sec: 20
  strict: false
char_tolerance: 20
concurrency: 0
max_retries: 3
request_timeout_sec: 300
output:
  dir: .
  num: 1
providers:
  deepseek:
    base_url: https://api.deepseek.com
    api_mode: chat
    model: deepseek-chat
    model_reasoning_effort: ""
    thinking_fallback:
      enabled: true
      attempt: 3
      model: deepseek-reasoner
```

翻译固定使用 `providers.deepseek`（与生成共享同一 DeepSeek 配置与 key）。
`char_tolerance` 用于字符数校验容差（默认 20）：若规则只有 `max`，则放宽为 `(-inf,max+20]`；若规则同时有 `min/max`，则放宽为 `[min-20,max+20]`。

## 校验与容差

- 规则区间：规则文件里定义的原始区间（例如 `max_chars=200` 或 `min/max=230/320`）。
- 容差区间：在规则区间基础上按 `char_tolerance` 放宽后的区间。
- 放行策略：
  - 命中规则区间：直接通过。
  - 未命中规则区间，但命中容差区间：通过，并输出 `校验提示`。
  - 未命中容差区间：判失败并重试。

示例：

- 标题 `max=200`，`char_tolerance=20`：可接受 `<=220`。
- 五点 `min=230,max=320`，`char_tolerance=20`：可接受 `[210,340]`。

## 参数

```bash
--config        配置文件路径，默认 ~/.syl-listing/config.yaml
-o, --out       输出目录
-n, --num       每个需求文件生成候选数量
--concurrency   保留参数（当前版本不限制并发，传入值不生效）
--max-retries   最大重试次数
--provider      覆盖配置中的 provider（当前仅支持 deepseek）
--verbose       终端输出详细 NDJSON（机器友好）
--log-file      NDJSON 日志文件路径
-v, --version   版本
```

## 故障排查

- `文件不是 listing 需求格式（缺少首行标志）`：
  检查首个非空行是否为 `===Listing Requirements===`。
- `缺少规则文件`：
  检查规则中心发布资产是否存在，或检查 `~/.syl-listing/rules/` 本地缓存是否完整。
- `规则中心警告：...`：
  默认会先尝试规则中心同步；当 `rules_center.strict=false` 时会回退本地缓存继续运行；若要强制失败可设为 `strict=true`。
- `... 为空。先复制 .../.env.example 为 .../.env 并填写 key`：
  复制并填写 `.env`，确认变量名与 `config.yaml` 对齐。
- 生成慢或超时：
  降低 `max_retries`，调整 `request_timeout_sec`，或切换更快模型。
- 翻译失败：
  检查 `providers.deepseek` 与 `DEEPSEEK_API_KEY` 是否正确。

## 退出码与自动化集成

- 全部成功：退出码 `0`。
- 只要有失败（部分失败/全部失败）：退出码 `1`。
- 默认输出人类可读进度，`--verbose` 输出 NDJSON，适合脚本解析。

## 安全与成本提示

- `.env` 含密钥，不要提交到仓库。
- `--verbose` 可能包含 system/user prompt 与模型返回文本，注意日志脱敏。
- 模型与翻译调用会产生费用；建议按需设置重试次数与超时。

## 自动发布

- 已内置 GoReleaser 配置：`.goreleaser.yml`
- 已内置 GitHub Actions 发布流：`.github/workflows/release.yml`
- 触发方式：推送 tag（`v*`）或手动触发 `release` workflow
- 发布产物：GitHub Release + checksums + Homebrew Formula + Scoop Manifest
- 需要的仓库密钥：
  - `HOMEBREW_TAP_GITHUB_TOKEN`
  - `SCOOP_BUCKET_GITHUB_TOKEN`
