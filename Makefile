.PHONY: test vet build tidy proto

test:
	go test ./...

vet:
	go vet ./...

build:
	go build ./...

tidy:
	go mod tidy
	cd api && go mod tidy
	cd contrib/exprlang && go mod tidy
	cd contrib/goscripts && go mod tidy
	cd store/ent && go mod tidy
	cd store/gorm && go mod tidy

# 生成 api/gen/go（需要 buf + protoc-gen-go / protoc-gen-go-grpc / protoc-gen-go-http）
proto:
	cd api && buf dep update
	cd api && buf generate
