.PHONY: test vet build tidy

test:
	go test ./...

vet:
	go vet ./...

build:
	go build ./...

tidy:
	go mod tidy
	cd contrib/exprlang && go mod tidy
	cd contrib/goscripts && go mod tidy
	cd store/ent && go mod tidy
	cd store/gorm && go mod tidy
