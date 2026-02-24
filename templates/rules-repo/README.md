# __RULES_REPO_NAME__

独立规则中心仓库（仅管理 `syl-listing` 的规则文件与发布产物）。

## 目录结构

- `rules/`：规则源文件（`title.yaml`、`bullets.yaml`、`description.yaml`、`search_terms.yaml`）
- `scripts/build_bundle.sh`：打包脚本，生成 `dist/rules-bundle.tar.gz`
- `.github/workflows/release.yml`：发布流程，自动把规则包上传到 GitHub Releases

## 本地打包

```bash
./scripts/build_bundle.sh
```

输出：

- `dist/rules-bundle.tar.gz`

## 发布规则

```bash
git tag rules-v2026.03.01
git push origin rules-v2026.03.01
```

CI 会执行：

1. 校验 4 个规则文件是否存在
2. 打包 `rules-bundle.tar.gz`
3. 创建 GitHub Release 并上传规则包资产

## 客户端配置

在 `syl-listing` 的 `~/.syl-listing/config.yaml` 中：

```yaml
rules_center:
  enabled: true
  owner: hooziwang
  repo: syl-listing-rules
  release: latest
  asset: rules-bundle.tar.gz
  timeout_sec: 20
  strict: false
```

