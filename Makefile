run:
	go run cmd/main.go

test:
	go test ./internal/...

build:
	go build -o bin/redis-server cmd/main.go
