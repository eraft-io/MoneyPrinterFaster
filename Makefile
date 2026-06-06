APP_NAME := moneyprinterFaster
VERSION  := 0.1.0
BUILD_DIR := ./build
LDFLAGS  := -ldflags "-X main.Version=$(VERSION)"

.PHONY: all build run clean test fmt vet tidy

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
