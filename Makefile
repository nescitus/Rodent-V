EXE = rodent_v

ifeq ($(OS),Windows_NT)
	EXE := $(EXE).exe
endif

.PHONY: all build clean windows linux

all: build

# Default build (uses host OS)
build:
	GOAMD64=v3 go build -ldflags="-s -w" -o $(EXE)

# Cross-compile for Windows (AVX2)
windows:
	GOOS=windows GOARCH=amd64 GOAMD64=v3 go build -ldflags="-s -w" -o rodent_v.exe

# Cross-compile for Linux (AVX2)
linux:
	GOOS=linux GOARCH=amd64 GOAMD64=v3 go build -ldflags="-s -w" -o rodent_v

clean:
	rm -f rodent_v rodent_v.exe
