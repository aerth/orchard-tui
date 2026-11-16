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
	orchard-io <<<"for our TUI, use edit_create to update docs/helpflags.md based on ./flags.txt file. use nice markdown."
genhelpslash:
	orchard-io <<<"for our TUI, use edit_create to update docs/helpslash.md based on ./slashcommands.go file. use nice markdown."
genhelp: gen genhelpflags genhelpslash

docker:
	test ! -d tmp/otools || git -C tmp/otools pull origin master
	test -d tmp/otools/.git || git clone https://github.com/aerth/orchard-tools tmp/otools
	docker build --network=host -t orchard:tui .
	@echo built container
	@test -f ~/.config/orchard/env || echo create ~/.config/orchard/env file to run quick docker test
	@test ! -f ~/.config/orchard/env || echo docker run --network=host \
		-v /etc/ssl/certs/ca-certificates.crt:/etc/ssl/certs/ca-certificates.crt:ro \
		-v ./tmp/data:/home/username:rw \
		-v ${HOME}/src/otools:/home/username/src/otools:ro \
		--hostname "orchard-os" \
		--env-file ~/.config/orchard/env \
		--rm \
		-a STDIN -a STDOUT -a STDERR \
		-it \
		orchard:tui <<<"Tell me about this machine, environment. What programs are installed, etc"
