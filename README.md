# NDisk

一个轻量、安全的自托管文件管理与分享服务。

## 功能特性

- 📁 **文件管理** — 上传、下载、重命名、移动、删除文件和文件夹
- 🔗 **安全分享** — 一键生成分享链接，支持密码保护和有效期设置
- 📤 **断点续传** — 基于 Resumable.js 的大文件分片上传，支持断点续传
- 🖥️ **NFS 挂载** — 通过 NFSv3 协议将网盘挂载为本地磁盘
- 👤 **多用户** — 支持多用户注册、登录，独立的文件空间
- 🔒 **安全认证** — JWT Token 认证，登录频率限制

## 技术栈

- **后端**: Go (Gin 框架)
- **前端**: HTML 模板 + Tailwind CSS + vanilla JavaScript
- **存储**: 本地文件系统 + SQLite
- **协议**: HTTP REST API + NFSv3

## 快速开始

### 前置要求

- Go 1.21+
- Make (可选)

### 编译与运行

```bash
# 克隆仓库
git clone https://github.com/Full-finger/NDisk.git
cd NDisk

# 复制配置文件并修改
cp config.example.toml config.toml

# 运行
make run
```

### 配置说明

编辑 `config.toml` 进行配置，参考 `config.example.toml` 获取完整配置项说明。

## 项目结构

```
NDisk/
├── cmd/server/          # 程序入口
├── internal/
│   ├── auth/            # 用户认证与授权
│   ├── config/          # 配置管理
│   ├── database/        # 数据库操作
│   ├── file/            # 文件管理逻辑
│   ├── nfs/             # NFS 服务
│   ├── share/           # 分享功能
│   ├── storage/         # 存储后端
│   └── web/             # Web 路由与模板渲染
├── web/
│   ├── static/js/       # 前端 JavaScript
│   └── templates/       # HTML 模板
├── config.example.toml  # 示例配置文件
└── Makefile
```

## 许可证

本项目源码参见 [GitHub 仓库](https://github.com/Full-finger/NDisk)。