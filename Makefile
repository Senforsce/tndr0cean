install: build
	@go install ./cmd/nw

build:
	@go build ./cmd/nw

example: 
	@go run example/cmd/main.go