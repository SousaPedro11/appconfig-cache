APP_NAME := appconfig-cache
DIST_DIR := dist
LAMBDA_BIN := bootstrap

.PHONY: tidy fmt test build-local run-local build-lambda clean setup

tidy:
	go mod tidy

fmt:
	gofmt -w ./cmd ./internal

test:
	go test ./...

build-local:
	mkdir -p $(DIST_DIR)
	go build -o $(DIST_DIR)/$(APP_NAME)-local ./cmd/local

build-server:
	mkdir -p $(DIST_DIR)
	go build -o $(DIST_DIR)/$(APP_NAME)-server ./cmd/server

run-local:
	AWS_SDK_LOAD_CONFIG=1 AWS_EC2_METADATA_DISABLED=true go run ./cmd/local -application=$(APP) -environment=$(ENV) -profile=$(PROFILE)

run-server:
	AWS_SDK_LOAD_CONFIG=1 AWS_EC2_METADATA_DISABLED=true HTTP_ADDR=$(or $(ADDR),:8080) go run ./cmd/server

build-lambda:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o $(DIST_DIR)/$(LAMBDA_BIN) ./cmd/lambda
	cd $(DIST_DIR) && zip -q lambda.zip $(LAMBDA_BIN)

clean:
	rm -rf $(DIST_DIR)

setup:
	go install github.com/evilmartians/lefthook@latest
	go install golang.org/x/tools/cmd/goimports@latest
	$(shell go env GOPATH)/bin/lefthook install

