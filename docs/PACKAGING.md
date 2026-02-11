# 包管理器发布说明

本文件说明 `brew` / `winget` 分发链路的维护方式。

## 目标

- macOS / Linux：通过 Homebrew 安装
- Windows：通过 winget 安装

## 相关脚本

- `scripts/generate-homebrew-formula.sh`
- `scripts/generate-winget-manifests.sh`

它们都以 `SHA256SUMS.txt` 作为单一校验数据来源，避免手工复制 hash。

## 相关工作流

- `.github/workflows/publish-homebrew.yml`
- `.github/workflows/publish-winget.yml`

触发方式：

- `release.published`
- `workflow_dispatch`（手动输入 tag）

## 手动生成（本地）

Homebrew Formula：

```bash
scripts/generate-homebrew-formula.sh \
  --tag v0.4.0 \
  --repo mingesthq/media-ingest \
  --checksums artifacts/SHA256SUMS.txt \
  --output out/mingest.rb
```

winget manifests：

```bash
scripts/generate-winget-manifests.sh \
  --tag v0.4.0 \
  --repo mingesthq/media-ingest \
  --checksums artifacts/SHA256SUMS.txt \
  --output-dir out
```

## 自动 PR（可选）

如果配置了 secrets，工作流会自动创建 PR；未配置时会上传生成结果作为 artifact。

Homebrew secrets：

- `HOMEBREW_TAP_GH_TOKEN`
- `HOMEBREW_TAP_REPO`（可选，默认 `mingesthq/homebrew-tap`）

winget secrets：

- `WINGET_GH_TOKEN`
- `WINGET_FORK_REPO`（例如 `yourname/winget-pkgs`）

