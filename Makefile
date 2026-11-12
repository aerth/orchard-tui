GIT_TAG := $(shell git tag | sort -V | tail -n 1)
GIT_COMMIT := $(shell git rev-parse HEAD)
XLDFLAGS=-X main.version=${GIT_TAG} -X main.gitcommit=${GIT_COMMIT}
XGOFLAGS=-v -trimpath
build:
	go build ${XGOFLAGS} -ldflags="${XLDFLAGS}" -v -o bin/ ./...
	go test -c -v -o bin/ ./...
	# compiled successfully! start with:
	# bin/orchard-tui -db history.db 2>./orchard.log
	ls -l bin/*
clean:
	rm -rf bin
run: build
	bin/orchard-tui --version
gen: build
	bin/orchard-tui -h |& tail -n +2 >flags.txt
install:
	install bin/orchard-tui ~/bin/
genhelpflags:
	orchard-io <<<"use edit_create to update docs/helpflags.md based on ./flags.txt file"
genhelpslash:
	orchard-io <<<"use edit_create to update docs/helpslash.md based on ./slashcommands.go file"
