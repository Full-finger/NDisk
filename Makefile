.PHONY: run build test clean

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

init:
	mkdir data

test:
	go test ./...

# 从 config.toml 解析数据库配置（仅非注释行）
DB_DRIVER := $(shell grep -E '^driver\s*=' config.toml | head -1 | sed 's/.*= *"\(.*\)"/\1/')
DB_DSN    := $(shell grep -E '^dsn\s*=' config.toml | head -1 | sed 's/.*= *"\(.*\)"/\1/')

# PostgreSQL 连接参数提取
PG_HOST   := $(shell echo "$(DB_DSN)" | sed -n 's/.*host=\([^ ]*\).*/\1/p')
PG_PORT   := $(shell echo "$(DB_DSN)" | sed -n 's/.*port=\([^ ]*\).*/\1/p')
PG_USER   := $(shell echo "$(DB_DSN)" | sed -n 's/.*user=\([^ ]*\).*/\1/p')
PG_PASS   := $(shell echo "$(DB_DSN)" | sed -n 's/.*password=\([^ ]*\).*/\1/p')
PG_DBNAME := $(shell echo "$(DB_DSN)" | sed -n 's/.*dbname=\([^ ]*\).*/\1/p')

clean:
	@if [ ! -f config.toml ]; then echo "错误: config.toml 不存在"; exit 1; fi
	@echo "⚠️  警告: 此操作将删除所有构建产物和数据库数据！"
	@echo "数据库驱动: $(DB_DRIVER)"
	@if [ "$(DB_DRIVER)" = "postgres" ]; then \
		echo "目标数据库: $(PG_DBNAME) @ $(PG_HOST):$(PG_PORT)"; \
		echo "数据库用户: $(PG_USER)"; \
	fi
	@echo ""
	@echo "此操作不可逆！确定要继续吗？请输入 yes 继续："
	@read confirm; \
	if [ "$$confirm" != "yes" ]; then echo "已取消。"; exit 1; fi
	rm -rf bin/* data/*
	@if [ "$(DB_DRIVER)" = "postgres" ]; then \
		echo "正在重建 PostgreSQL 数据库 $(PG_DBNAME)..."; \
		PGPASSWORD=$(PG_PASS) psql -h $(PG_HOST) -p $(PG_PORT) -U $(PG_USER) -d postgres -c "DROP DATABASE IF EXISTS $(PG_DBNAME);" && \
		PGPASSWORD=$(PG_PASS) psql -h $(PG_HOST) -p $(PG_PORT) -U $(PG_USER) -d postgres -c "CREATE DATABASE $(PG_DBNAME);" && \
		echo "数据库重建完成。" || echo "错误: 数据库重建失败，请检查连接配置。"; \
	elif [ "$(DB_DRIVER)" = "sqlite" ]; then \
		echo "SQLite 数据文件已随 data/* 删除。"; \
	else \
		echo "错误: 不支持的数据库驱动: $(DB_DRIVER)"; \
	fi