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
	go mod tidy && go mod vendor && go build -o ${PACKAGEPATH}/bcs-image-proxy ./cmd/accelerboat/main.go