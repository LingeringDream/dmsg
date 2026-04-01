# DMSG - Decentralized Message Node

DMSG 是一个基于 Go 语言构建的轻量级去中心化消息传递节点。它利用 Libp2p 进行网络通信，支持 Wasm 插件扩展，并使用 BadgerDB 进行数据持久化。

## ✨ 特性
- 🌐 **完全去中心化**: 基于 Libp2p 协议栈实现 P2P 通信与节点发现。
- 🔌 **Wasm 插件支持**: 支持通过 WebAssembly 扩展节点业务逻辑。
- 💾 **高效存储**: 集成 BadgerDB，提供高性能的键值存储与消息持久化。
- 🚀 **轻量级设计**: 优化的依赖关系与无外部配置的运行模式。

## 🛠 安装与编译
确保已安装 Go 1.21+。

```bash
# 克隆仓库
git clone https://github.com/LingeringDream/dmsg.git
cd dmsg

# 安装依赖并编译
go build -o dmsg ./cmd/dmsg
```

## 📦 部署与运行
编译成功后，可直接运行二进制文件。

```bash
# 前台运行
./dmsg

# 后台运行 (Linux)
nohup ./dmsg > dmsg.log 2>&1 &
```

## 📁 项目结构
```text
.
├── cmd/dmsg/main.go       # 程序入口
├── internal/
│   ├── net/p2p.go         # P2P 网络层实现
│   ├── plugin/wasm.go     # Wasm 插件管理器
│   ├── store/store.go     # 数据存储层 (BadgerDB)
│   └── trust/eigen.go     # 信任与共识逻辑
├── go.mod
└── go.sum
```

## 📜 License
MIT License
