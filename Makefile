.PHONY: all clean

TARGET := webhook

all: build

build: init
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o $(TARGET) .

init:
	@if [ ! -f go.mod ]; then \
		go mod init $(MODULE_NAME); \
	fi

clean:
	rm -f $(TARGET) go.mod
