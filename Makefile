
bin/viampet: *.go cmd/module/*.go *.mod
	-mkdir bin
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
