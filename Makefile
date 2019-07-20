OUT_PATH :=
OUT_WORKER := cli_worker_linux
OUT_HTTPD:= cli_httpd_linux
OUT_PREPROCESSOR := cli_preprocessor_linux
PKG := github.com/xf0e/open-ocr
VERSION := $(shell git describe --always --long --dirty)
SHA1VER := $(shell git rev-parse HEAD)
DATE := $(shell date +'%Y-%m-%d_%T')
PKG_LIST := $(shell go list ${PKG}/... | grep -v /vendor/)
GO_FILES := $(shell find . -name 'main.go' | grep -v /vendor/)

#-buildmode=pie -a -tags netgo -ldflags="-s -w -X main.sha1ver=$GIT_COMMIT

all: run

server:
	go build -o ${OUT_WORKER} -buildmode=pie -a -tags netgo -ldflags="-s -w -X main.buildTime=${DATE} \
	 -X main.sha1ver=${SHA1VER} -X main.version=${VERSION}" cli-worker/main.go
	go build -o ${OUT_HTTPD} -buildmode=pie -a -tags netgo -ldflags="-s -w -X main.buildTime=${DATE} \
	 -X main.sha1ver=${SHA1VER} -X main.version=${VERSION}" cli-httpd/main.go
	go build -o ${OUT_PREPROCESSOR} -buildmode=pie -a -tags netgo -ldflags="-s -w -X main.buildTime=${DATE} \
	 -X main.sha1ver=${SHA1VER} -X main.version=${VERSION}" cli-preprocessor/main.go

#test:
#	@go test -short ${PKG_LIST}
#
#vet:
#	@go vet ${PKG_LIST}
#
#lint:
#	@for file in ${GO_FILES} ;  do \
#		golint $$file ; \
#	done
#
#static: vet lint
#	go build -i -v -o ${OUT}-v${VERSION} -tags netgo -ldflags="-extldflags \"-static\" -w -s -X main.version=${VERSION}" ${PKG}

run: server
	./${OUT_WORKER}
	./${OUT_HTTPD}
	./${OUT_PREPROCESSOR}

clean:
	-@rm ${OUT_WORKER} ${OUT_HTTPD} ${OUT_PREPROCESSOR}

.PHONY: run server static vet lint
