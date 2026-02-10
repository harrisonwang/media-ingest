# Mingest

为 AI 时代设计的视频归档工具。

把你有权保存的视频下载到本地，自动整理成可长期保存、可检索、可复用的资料。

## 适合谁

- 内容创作者：批量保存素材，统一转成剪辑友好的格式
- 学习/研究：把课程、公开资料离线归档，防止链接失效
- 团队知识库：把内部培训/宣发视频统一落盘，便于归档与复查

## 能做什么

- 一条命令下载并合并为 `mp4`，自动写入元数据与封面
- 需要登录/会员/额外验证（例如年龄确认）时，提供 `mingest auth <platform>` 一次性登录
- 自动维护 cookies 缓存：优先使用缓存；必要时从浏览器刷新账户登录信息
- Windows 下 Chrome 读取 cookies 失败时，自动尝试 CDP（由浏览器进程内导出明文 cookies）

## 开始使用

```bash
mingest get "<url>"
```

需要登录时：

```bash
mingest auth youtube
mingest auth bilibili
```

## 合规与边界（请务必阅读）

- Mingest **不提供任何内容**，不提供在线解析/代下服务
- Mingest 仅用于下载/归档你拥有版权或已获授权、或在法律与平台规则允许范围内可保存的内容
- Mingest 不支持绕过 DRM 等技术保护措施

详细条款：

- `.docs/disclaimer.md`
- `.docs/acceptable-use.md`
- `.docs/privacy.md`

