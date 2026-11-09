GIT_TAG := $(shell git tag | sort -V | tail -n 1)
GIT_COMMIT := $(shell git rev-parse HEAD)
XLDFLAGS=-X main.version=${GIT_TAG} -X main.gitcommit=${GIT_COMMIT}
XGOFLAGS=-v -trimpath
build:
	go build ${XGOFLAGS} -ldflags="${XLDFLAGS}" -v -o bin/ ./...
	go test -c -v -o bin/ ./...
	ls -l bin/*
	# try:
	# bin/orchard-tui -db history.db 2>./orchard.log
clean:
	rm -rf bin
run: build
gen: build
	bin/orchard-tui -h |& tail -n +2 >flags.txt
install:
	install bin/orchard-tui ~/bin/
