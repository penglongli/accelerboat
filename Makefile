# default config
MAKE:=make

# init the build information
ifdef HASTAG
	GITTAG=${HASTAG}
else
	GITTAG=$(shell git describe --always)
endif

VERSION=${GITTAG}-$(shell date +%y.%m.%d)

# build path config
export PACKAGEPATH=./build/accelerboat.${VERSION}

.PHONY: build
build:
	mkdir -p ${PACKAGEPATH}
	go mod tidy && go mod vendor && go build -o ${PACKAGEPATH}/accelerboat ./cmd/accelerboat/main.go

.PHONY: build-cli
build-cli:
	mkdir -p ${PACKAGEPATH}
	go mod tidy && go mod vendor && go build -o ${PACKAGEPATH}/accelerboat-cli ./cmd/cli/

.PHONY: build-image
build-image:
	mkdir -p ${PACKAGEPATH}
	go mod tidy && go mod vendor && go build -o ${PACKAGEPATH}/accelerboat ./cmd/accelerboat/main.go
	upx -9 ${PACKAGEPATH}/accelerboat
	cp Dockerfile ${PACKAGEPATH}/
	cd ${PACKAGEPATH} && docker build -t accelerboat:latest .
