


bin/viampet: bin *.go cmd/module/*.go *.mod
	go build -o bin/viampet cmd/module/cmd.go

test:
	go test

lint:
	gofmt -w -s .

updaterdk:
	go get go.viam.com/rdk@latest
	go mod tidy

module: bin/viampet
	tar czf module.tar.gz bin/viampet meta.json

bin:
	-mkdir bin
