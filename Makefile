GIT_TAG := $(shell git tag | sort -V | tail -n 1)
GIT_COMMIT := $(shell git rev-parse HEAD)
LDFLAGS=-X main.version=${GIT_TAG} -X main.gitcommit=${GIT_COMMIT}
build:
	go build -v -trimpath -ldflags="${LDFLAGS}" -v -o bin/ ./...
	go test -c -v -o bin/ ./...
clean:
	rm -rf bin
run: build
	echo bin/orchard-tui -db history.db 2>./orchard.log
gen: build
	bin/orchard-tui -h |& tail -n +2 >flags.txt
install:
	install bin/orchard-tui ~/bin/
