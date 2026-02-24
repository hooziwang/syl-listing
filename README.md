# syl-listing

`syl-listing` 是一个基于 Go + Cobra 的 CLI，用于把 Listing 需求 Markdown 批量生成为英文 Listing Markdown 文件。

## 命令

```bash
syl-listing [file_or_dir ...]
syl-listing gen [file_or_dir ...]
syl-listing version
```

## 自动初始化

首次运行会自动创建：

- `~/.syl-listing/config.yaml`
- `~/.syl-listing/rules/`（分段规则文件目录）
- `~/.syl-listing/.env.example`

并要求手动创建：

- `~/.syl-listing/.env`

## 规则文件（分段）

规则目录默认：`~/.syl-listing/rules`

- `title.md`：标题规则
- `bullets.md`：五点规则
- `description.md`：描述规则
- `search_terms.md`：搜索词规则

模型在每一步仅加载对应规则文件。

## 输入识别

- 输入可为单文件、多文件或目录。
- 目录递归扫描，仅处理 `.md`。
- 需求文件首个非空行必须是：

```text
===Listing Requirements===
```

## 生成流程（仅英文）

按 4 个步骤生成并校验：

1. `title`
2. `bullets`
3. `description`
4. `search_terms`

`关键词` 与 `分类` 直接复制输入，不由模型生成。

## 输出

每个候选生成 1 个文件：

- `listing_xxxxxxxx_en.md`

规则：

- `xxxxxxxx` 为 8 位随机串（数字 + 大小写字母）。
- 冲突时自动重试生成新随机串。

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
