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

.PHONY: build

build: modules linux windows macos

windows:
	$(EXPORT) GOOS=windows&& go build -o $(OUTPUT_PATH)/windows/$(BINARY_NAME).exe ./...

linux:
	$(EXPORT) GOOS=linux&& go build -o $(OUTPUT_PATH)/linux/$(BINARY_NAME) ./...

macos:
	$(EXPORT) GOOS=darwin&& go build -o $(OUTPUT_PATH)/macos/$(BINARY_NAME) ./...

modules:
	go mod tidy
	go mod download