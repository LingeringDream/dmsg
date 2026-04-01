# Changelog

所有重要的项目变更将记录在此文件中。

## [1.0.0] - 2026-04-01
### Added
- 基于 Libp2p 的去中心化消息路由功能。
- WASM 插件执行支持（基于 Wazero 引擎）。
- BadgerDB 本地持久化存储引擎。
- 完整的项目文档（README 与更新记录）。

### Fixed
- 修复了 BadgerDB 启动时的清单（Manifest）损坏问题。
- 彻底清理了 Git 历史中导致 GitHub 报错的大文件（`*.vlog`, `*.mem`）。
- 修正了 WASM 模块加载时的内存对齐与实例化错误。

### Security & Maintenance
- 更新了 `.gitignore`，严格排除运行时产生的敏感数据与日志。
- 优化了依赖项管理，移除了冗余的二进制文件追踪。
