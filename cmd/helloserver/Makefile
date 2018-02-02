.PHONY: module
module: helloserver

helloserver: helloserver.go
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' .

clean:
	rm helloserver


