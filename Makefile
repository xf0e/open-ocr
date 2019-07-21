OUT_PATH :=
OUT_WORKER := cli_worker_linux
OUT_HTTPD:= cli_httpd_linux
OUT_PREPROCESSOR := cli_preprocessor_linux
PKG := github.com/xf0e/open-ocr
VERSION := $(shell git describe --tags|sed -e "s/\-/\./g")
SHA1VER := $(shell git rev-parse HEAD)
DATE := $(shell date +'%Y-%m-%d_%T')
PKG_LIST := $(shell go list ${PKG}/... | grep -v /vendor/)
GO_FILES := $(shell find . -name 'main.go' | grep -v /vendor/)

all: run

release:
	go build -o ${OUT_WORKER} -buildmode=pie -a -tags netgo -ldflags="-s -w -X github.com/xf0e/open-ocr.buildTime=${DATE} \
	 -X github.com/xf0e/open-ocr.sha1ver=${SHA1VER} -X github.com/xf0e/open-ocr.version=${VERSION}" cli-worker/main.go
	go build -o ${OUT_HTTPD} -buildmode=pie -a -tags netgo -ldflags="-s -w -X main.buildTime=${DATE} \
	 -X main.sha1ver=${SHA1VER} -X main.version=${VERSION}" cli-httpd/main.go
	go build -o ${OUT_PREPROCESSOR} -buildmode=pie -a -tags netgo -ldflags="-s -w -X main.buildTime=${DATE} \
	 -X main.sha1ver=${SHA1VER} -X main.version=${VERSION}" cli-preprocessor/main.go

debug:
	go build -o ${OUT_WORKER} -buildmode=pie -a -tags netgo -ldflags="-w -X github.com/xf0e/open-ocr.buildTime=${DATE} \
	 -X github.com/xf0e/open-ocr.sha1ver=${SHA1VER} -X github.com/xf0e/open-ocr.version=${VERSION}" cli-worker/main.go
	go build -o ${OUT_HTTPD} -buildmode=pie -a -tags netgo -ldflags="-w -X main.buildTime=${DATE} \
	 -X main.sha1ver=${SHA1VER} -X main.version=${VERSION}" cli-httpd/main.go
	go build -o ${OUT_PREPROCESSOR} -buildmode=pie -a -tags netgo -ldflags="-w -X main.buildTime=${DATE} \
	 -X main.sha1ver=${SHA1VER} -X main.version=${VERSION}" cli-preprocessor/main.go

test:
	@go test -v ${PKG}

vet:
	@go vet ${PKG_LIST}

lint:
	@for file in ${GO_FILES} ;  do \
		/home/grrr/go/bin/golint $$file ; \
	done

static: vet lint
	go build -i -v -o ${OUT}-v${VERSION} -tags netgo -ldflags="-extldflags \"-static\" -w -s -X main.version=${VERSION}" ${PKG}

run: release
	./${OUT_WORKER}
	./${OUT_HTTPD}
	./${OUT_PREPROCESSOR}

clean:
	-@rm ${OUT_WORKER} ${OUT_HTTPD} ${OUT_PREPROCESSOR}

.PHONY: run release static vet lint
