build:
    #!/usr/bin/env sh
    set -eu
    mkdir -p bin
    version="dev-$(git rev-parse --short HEAD)"
    if test -n "$(git status --porcelain)"; then
        version="$version+"
    fi
    go build -trimpath -ldflags="-X github.com/toppk/omni/internal/app.Version=$version" -o bin/omni ./cmd/omni

