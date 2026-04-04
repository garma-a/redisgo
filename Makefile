run:
	go run cmd/redis-server/main.go

test:
	go test ./internal/...

build:
	go build -o bin/redis-server cmd/redis-server/main.go
