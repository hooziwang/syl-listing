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
- `~/.syl-listing/rules/`（仅目录，不生成默认规则文件）
- `~/.syl-listing/.env.example`

并要求手动创建：

- `~/.syl-listing/.env`

## 规则文件（分段 + 结构化）

规则目录默认：`~/.syl-listing/rules`

- `title.yaml`
- `bullets.yaml`
- `description.yaml`
- `search_terms.yaml`

每一步只加载对应规则文件。每个规则文件同时包含：

- `instruction`：给模型的规则描述
- `constraints` / `output`：给程序校验的结构化约束

程序直接把该规则文件原文作为 `system`，并解析同一文件做校验，不做二次拼接。
`~/.syl-listing/rules` 是唯一规则定义源；缺少任一规则文件会直接报错。

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
char_tolerance: 20
concurrency: 0
max_retries: 3
request_timeout_sec: 300
output:
  dir: .
  num: 1
translation:
  provider: deepseek
  base_url: https://api.deepseek.com
  model: deepseek-chat
  api_key_env: DEEPSEEK_API_KEY
  source: en
  target: zh
providers:
  deepseek:
    base_url: https://api.deepseek.com
    api_mode: chat
    model: deepseek-chat
    model_reasoning_effort: ""
  openai:
    base_url: https://flux-code.cc
    api_mode: responses
    model: gpt-5.3-codex
    model_reasoning_effort: high
```

翻译配置可在 `translation` 节点覆盖；当前支持 `tencent_tmt` 和 `deepseek`。
`char_tolerance` 用于字符数校验容差（默认 20）：若规则只有 `max`，则放宽为 `(-inf,max+20]`；若规则同时有 `min/max`，则放宽为 `[min-20,max+20]`。

## 参数

```bash
--config        配置文件路径，默认 ~/.syl-listing/config.yaml
-o, --out       输出目录
-n, --num       每个需求文件生成候选数量
--concurrency   保留参数（当前版本不限制并发）
--max-retries   最大重试次数
--provider      覆盖配置中的 provider
--verbose       终端输出详细 NDJSON（机器友好）
--log-file      NDJSON 日志文件路径
-v, --version   版本
```
