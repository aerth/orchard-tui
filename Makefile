build:
	go build -v -o bin/ ./...
	go test -c -v -o bin/ ./...
clean:
	rm -rf bin
run: build
	echo bin/orchard-tui -db history.db 2>./orchard.log

