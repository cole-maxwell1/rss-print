APP_NAME := rss-print
TAILWIND_VERSION := v4.3.0
OS := $(shell uname -s | tr A-Z a-z)
ifeq ($(OS),darwin)
	OS=macos
endif
ARCH := $(shell uname -m)

ifeq ($(ARCH),x86_64)
	ARCH=x64
else ifeq ($(ARCH),aarch64)
	ARCH=arm64
else ifeq ($(ARCH),arm64)
	ARCH=arm64
endif

TAILWIND_URL := https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-$(OS)-$(ARCH)
TAILWIND_CLI := bin/tailwindcss

.PHONY: all build clean setup tailwind run

all: clean setup tailwind build

setup:
	@mkdir -p bin
	@if [ ! -f $(TAILWIND_CLI) ]; then \
		echo "Downloading Tailwind CLI..."; \
		curl -sLO $(TAILWIND_URL); \
		mv tailwindcss-$(OS)-$(ARCH) $(TAILWIND_CLI); \
		chmod +x $(TAILWIND_CLI); \
	fi

tailwind: setup
	$(TAILWIND_CLI) -i ./public/input.css -o ./ui/static/css/style.css --minify

build: tailwind
	go build -o $(APP_NAME) ./cmd/server

run: tailwind
	go run ./cmd/server

clean:
	rm -f $(APP_NAME)
	rm -rf bin
	rm -f ./ui/static/css/style.css
