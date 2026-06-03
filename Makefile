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
	@mkdir -p ui/static/fonts
	@if [ ! -f ui/static/fonts/work-sans-400.woff2 ]; then \
		echo "Downloading brand fonts (Work Sans, Poppins)..."; \
		curl -sLo ui/static/fonts/work-sans-400.woff2 https://cdn.jsdelivr.net/npm/@fontsource/work-sans/files/work-sans-latin-400-normal.woff2; \
		curl -sLo ui/static/fonts/work-sans-500.woff2 https://cdn.jsdelivr.net/npm/@fontsource/work-sans/files/work-sans-latin-500-normal.woff2; \
		curl -sLo ui/static/fonts/work-sans-600.woff2 https://cdn.jsdelivr.net/npm/@fontsource/work-sans/files/work-sans-latin-600-normal.woff2; \
		curl -sLo ui/static/fonts/poppins-600.woff2 https://cdn.jsdelivr.net/npm/@fontsource/poppins/files/poppins-latin-600-normal.woff2; \
		curl -sLo ui/static/fonts/poppins-700.woff2 https://cdn.jsdelivr.net/npm/@fontsource/poppins/files/poppins-latin-700-normal.woff2; \
	fi
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
