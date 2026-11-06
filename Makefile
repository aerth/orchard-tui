build:
	go build -v -o bin/ ./...
	go test -c -v -o bin/ ./...
clean:
	rm -rf bin
run: build
	bin/orchard-tui 2>./orchard.log

