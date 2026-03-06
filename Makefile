BINARY := tradefox
BUILD_DIR := ./build
CMD_DIR := ./cmd/tradefox

.PHONY: build run install test clean mock

build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) $(CMD_DIR)
	@echo "Built $(BUILD_DIR)/$(BINARY)"

run: build
	$(BUILD_DIR)/$(BINARY)

install:
	go install $(CMD_DIR)
	@echo "Installed $(BINARY) to GOPATH/bin"

test:
	go test ./...

clean:
	rm -rf $(BUILD_DIR)
	@echo "Cleaned build artifacts"

mock: build
	$(BUILD_DIR)/$(BINARY) --mock
