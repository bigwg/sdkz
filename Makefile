# sdkz 构建脚本
BINARY    := sdkz
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X sdkz/pkg/version.Version=$(VERSION)
OUTDIR    := dist

.PHONY: all build test lint clean install uninstall fmt vet

all: build

build:
	@mkdir -p $(OUTDIR)
	go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/$(BINARY) ./cmd/sdkz
	@cp $(OUTDIR)/$(BINARY) $(BINARY)

# 三平台交叉构建
build-all:
	@mkdir -p $(OUTDIR)
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/sdkz-linux-amd64   ./cmd/sdkz
	GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/sdkz-linux-arm64   ./cmd/sdkz
	GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/sdkz-darwin-amd64  ./cmd/sdkz
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/sdkz-darwin-arm64  ./cmd/sdkz
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/sdkz-windows-amd64.exe ./cmd/sdkz
	GOOS=windows GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/sdkz-windows-arm64.exe ./cmd/sdkz
	@pkg=$(CURDIR)/$(OUTDIR)/.pkg && rm -rf $$pkg && mkdir -p $$pkg && cd $(OUTDIR) && \
	for f in sdkz-linux-amd64 sdkz-linux-arm64 sdkz-darwin-amd64 sdkz-darwin-arm64 sdkz-windows-amd64.exe sdkz-windows-arm64.exe; do \
	  case $$f in \
	    *.exe) inner=sdkz.exe ;; \
	    *)     inner=sdkz ;; \
	  esac; \
	  cp $$f $$pkg/$$inner && tar -czf $$f.tar.gz -C $$pkg $$inner && rm -f $$pkg/$$inner; \
	done; \
	rm -rf $$pkg; \
	( command -v sha256sum >/dev/null && sha256sum sdkz-*.tar.gz || shasum -a 256 sdkz-*.tar.gz ) > checksums.txt && \
	echo "打包完成：$(ls sdkz-*.tar.gz | wc -l) 个平台"

test:
	go test ./...

# 快速本地验证（临时 SDKZ_DIR + 本地假源，不碰真实网络）
test-integration:
	go test ./pkg/service/... -run TestIntegration -v

fmt:
	gofmt -l -w .

vet:
	go vet ./...

lint: vet

install: build
	@echo "installed to $(OUTDIR)/$(BINARY)"

clean:
	rm -rf $(OUTDIR)
