# SPDX-License-Identifier: GPL-3.0-or-later
GO ?= go

.PHONY: build test integration clean
build:
	CGO_ENABLED=0 $(GO) build -buildvcs=false -trimpath -o silk ./cmd/silk
test:
	$(GO) test -buildvcs=false ./...
integration:
	SILK_INTEGRATION=1 $(GO) test -buildvcs=false -v ./internal/browser
clean:
	rm -f silk
