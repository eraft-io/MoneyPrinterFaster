APP_NAME := moneyprinterFaster
VERSION  := 0.1.0
BUILD_DIR := ./build
DOCKER_IMAGE := moneyprinter-faster
LDFLAGS  := -ldflags "-X main.Version=$(VERSION)"

.PHONY: all build run clean test fmt vet tidy docker docker-run docker-stop

all: tidy build

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...

build: tidy fmt vet
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) ./cmd/server

run:
	go run ./cmd/server

test:
	go test -v -race ./...

clean:
	rm -rf $(BUILD_DIR)
	rm -rf ./data

# 交叉编译
build-linux:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 ./cmd/server

build-darwin:
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 ./cmd/server
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 ./cmd/server

build-windows:
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe ./cmd/server

# Docker
docker:
	docker build -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest .
	docker tag $(DOCKER_IMAGE):latest $(DOCKER_IMAGE):latest
	@echo ""
	@echo "✅ Docker 镜像构建完成: $(DOCKER_IMAGE):$(VERSION)"
	@echo ""
	@echo "运行命令:"
	@echo "  make docker-run"
	@echo "  docker run -d -p 8080:8080 -v \$\$(pwd)/config.toml:/app/config.toml -v mpf-data:/app/data $(DOCKER_IMAGE)"

docker-run:
	docker run -d \
		--name moneyprinter \
		-p 8080:8080 \
		-v $(CURDIR)/config.toml:/app/config.toml \
		-v mpf-data:/app/data \
		--restart unless-stopped \
		$(DOCKER_IMAGE):latest
	@echo ""
	@echo "✅ 容器已启动: http://localhost:8080"
	@echo "查看日志: docker logs -f moneyprinter"
	@echo "停止容器: docker stop moneyprinter"

docker-stop:
	docker stop moneyprinter 2>/dev/null || true
	docker rm moneyprinter 2>/dev/null || true
	@echo "容器已停止并移除"
