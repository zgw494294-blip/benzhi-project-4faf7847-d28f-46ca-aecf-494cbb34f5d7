# BENZHI_README

基于 Go 实现的heritage-tree-relocation-permit Web 项目，一款后端服务，已完整实现古树迁移作业许可工作台，覆盖建档、双评估、方案修订、技术审查、整改复核、现场核验、内容冻结、许可签发和时间线查询，并使用摘要链事件日志与原子快照持久化。

## 项目说明
- 项目：benzhi-project-4faf7847-d28f-46ca-aecf-494cbb34f5d7
- 项目用途：已完整实现古树迁移作业许可工作台，覆盖建档、双评估、方案修订、技术审查、整改复核、现场核验、内容冻结、许可签发和时间线查询，并使用摘要链事件日志与原子快照持久化。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/server -selfcheck -addr=127.0.0.1:19081
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-4faf7847-d28f-46ca-aecf-494cbb34f5d7-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-4faf7847-d28f-46ca-aecf-494cbb34f5d7-arm64 linux/arm64
docker run -it benzhi-project-4faf7847-d28f-46ca-aecf-494cbb34f5d7-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -selfcheck -addr=127.0.0.1:19081`
