# NOFX Docker 快速参考卡

## 🚀 最常用命令

### 重启服务（应用 .env 更改）
```bash
cd D:\TradingAgent\nofx
docker-compose restart
```

### 重新构建并启动（应用代码更改）
```bash
cd nofx
docker-compose down
docker-compose build
docker-compose up -d

```

### 查看实时日志
```bash
docker-compose logs -f nofx
```

---

## 📋 完整命令列表

| 操作 | 命令 |
|------|------|
| **启动** | `docker-compose up -d` |
| **停止** | `docker-compose stop` |
| **重启** | `docker-compose restart` |
| **停止并删除** | `docker-compose down` |
| **重新构建** | `docker-compose build` |
| **构建并启动** | `docker-compose up -d --build` |
| **查看状态** | `docker-compose ps` |
| **查看日志** | `docker-compose logs -f` |
| **查看后端日志** | `docker-compose logs -f nofx` |
| **查看前端日志** | `docker-compose logs -f nofx-frontend` |
| **进入容器** | `docker exec -it nofx-trading sh` |

---

## ⚙️ 修改配置后的操作

### 修改了 .env 文件
```bash
# 只需重启，不需要重新构建
docker-compose restart
```

### 修改了 Go 代码
```bash
# 必须重新构建
cd nofx
docker-compose down
docker-compose build
docker-compose up -d

```

### 修改了 docker-compose.yml
```bash
# 重新创建容器
docker-compose down
docker-compose up -d
```

---

## 🔍 验证 OKX API

### 1. 检查环境变量
```bash
# 确认 USE_OKX_API=true
grep USE_OKX_API nofx/.env
```

### 2. 查看日志
```bash
# 查找 "使用 OKX API" 字样
docker-compose logs nofx | grep -i okx
```

### 3. 检查健康状态
```bash
curl http://localhost:8080/api/health
```

---

## 🛠️ 故障排查

### 容器无法启动
```bash
# 查看详细错误
docker-compose logs nofx

# 检查端口占用
netstat -ano | findstr :8080
```

### 代码修改未生效
```bash
# 强制重新构建（不使用缓存）
docker-compose build --no-cache
docker-compose up -d
```

### 网络连接问题
```bash
# 进入容器测试
docker exec -it nofx-trading sh
wget -O- https://www.okx.com/api/v5/public/time
exit
```

---

## 📊 访问地址

- **前端**: http://localhost:8888
- **后端 API**: http://localhost:8080
- **健康检查**: http://localhost:8080/api/health

---

## 🎯 典型工作流程

### 启用 OKX API（首次）
```bash
# 1. 编辑 .env 文件
# 添加: USE_OKX_API=true

# 2. 重启容器
cd D:\TradingAgent\nofx
docker-compose restart

# 3. 验证日志
docker-compose logs -f nofx
```

### 更新代码后重启
```bash
# 1. 停止容器
docker-compose down

# 2. 重新构建
docker-compose build

# 3. 启动
docker-compose up -d

# 4. 查看日志
docker-compose logs -f
```

### 日常调试
```bash
# 查看实时日志
docker-compose logs -f nofx

# 查看最近 50 行
docker-compose logs --tail=50 nofx

# 查看特定时间段
docker-compose logs --since 10m nofx
```

---

## 💡 提示

1. **修改 .env 后只需 `restart`，不需要 `build`**
2. **修改 Go 代码后必须 `build`**
3. **使用 `-d` 参数后台运行，不加则前台运行**
4. **`down` 会删除容器但保留数据卷**
5. **`down -v` 会同时删除数据卷（慎用！）**
