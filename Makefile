BINARY_NAME=conch
OUTPUT_PATH=build
STACK_NAME=ecs-fargate-$(MODULE_NAME)
ifeq ($(OS),Windows_NT)
    EXE_SUFFIX=.exe
    EXPORT=set
else
	EXE_SUFFIX=
	EXPORT=export
endif

.PHONY: build test vendor

build: bin
	zip $(OUTPUT_PATH)/conch-windows $(OUTPUT_PATH)/windows/$(BINARY_NAME).exe
	zip $(OUTPUT_PATH)/conch-linux $(OUTPUT_PATH)/linux/$(BINARY_NAME)
	zip $(OUTPUT_PATH)/conch-macos $(OUTPUT_PATH)/macos/$(BINARY_NAME)

windows:
	$(EXPORT) GOOS=windows&& go build -mod=vendor -o $(OUTPUT_PATH)/windows/$(BINARY_NAME).exe ./...

linux:
	$(EXPORT) GOOS=linux&& go build -mod=vendor -o $(OUTPUT_PATH)/linux/$(BINARY_NAME) ./...

macos:
	$(EXPORT) GOOS=darwin&& go build -mod=vendor -o $(OUTPUT_PATH)/macos/$(BINARY_NAME) ./...

bin: windows linux macos

vendor:
	go mod tidy
	go mod vendor