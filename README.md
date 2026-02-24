# syl-listing

`syl-listing` 是一个基于 Go + Cobra 的 CLI，用于把 Listing 需求 Markdown 批量生成中英 Listing Markdown 文件。

## 命令

```bash
syl-listing [file_or_dir ...]
syl-listing gen [file_or_dir ...]
syl-listing version
```

## 自动初始化

首次运行会自动创建：

- `~/.syl-listing/config.yaml`
- `~/.syl-listing/rules/`（分段规则目录）
- `~/.syl-listing/.env.example`

并要求手动创建：

- `~/.syl-listing/.env`

## 规则文件（分段）

规则目录默认：`~/.syl-listing/rules`

- `title.md`
- `bullets.md`
- `description.md`
- `search_terms.md`

每一步只加载对应规则文件。模型侧只负责英文生成。

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

## 配置文件示例

```yaml
provider: openai
api_key_env: SYL_LISTING_API_KEY
rules_dir: ~/.syl-listing/rules
concurrency: 0
max_retries: 3
request_timeout_sec: 300
output:
  dir: .
  num: 1
providers:
  openai:
    base_url: https://flux-code.cc
    api_mode: responses
    model: gpt-5.3-codex
    model_reasoning_effort: high
```

翻译配置可在 `translation` 节点覆盖；当前仅支持 `tencent_tmt`。

## 参数

```bash
--config        配置文件路径，默认 ~/.syl-listing/config.yaml
-o, --out       输出目录
-n, --num       每个需求文件生成候选数量
--concurrency   保留参数（当前版本不限制并发）
--max-retries   最大重试次数
--provider      覆盖配置中的 provider
--log-file      NDJSON 日志文件路径（默认 stdout）
-v, --version   版本
```
