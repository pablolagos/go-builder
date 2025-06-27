# 🧱 go-builder — Declarative Go builds made simple

> Because `go build` shouldn't turn into a puzzle.

When you're building Go projects, it usually starts simple:

```bash
go build ./cmd/myapp
```

But then things get more serious…

* You add **tags**, **CGO settings**, and **cross-compilation** targets
* You inject **version info** with `-X` flags
* You enable **static builds** with linker flags
* You need to **compile inside Docker** to match production
* And before you know it, your build command is a 200-character monster

So you write a `Makefile`:

```makefile
build:
	GOOS=linux GOARCH=arm64 \
	CGO_ENABLED=1 CC=GCC \
	go build -ldflags "-s -w -X main.version=$(VERSION) -linkmode external -extldflags '-static'" \
	-tags "prod" -o builds/linux/arm64/myapp ./cmd/myapp
```

or even worse:

```makefile
docker-build:
	docker run --rm -v "$$(pwd)":/usr/src/myapp -w /usr/src/myapp \
    	-e GIT_TERMINAL_PROMPT=0 -e "GOPRIVATE=github.com/acme_inc/*" docker.io/golang:1.23-alpine sh -c "\
    	apk add --no-cache musl-dev build-base git autoconf automake libtool  && \
    	git config --global url.https://$(GIT_TOKEN):x-oauth-basic@github.com/.insteadOf https://github.com/ && \
    	cd /usr/src/myapp && \
    	export CGO_ENABLED=1 CC=gcc CGO_LDFLAGS='-L/usr/local/lib -lm' && \
    	go build --tags \"linux\" -ldflags='-linkmode external -extldflags \"-static\" -s -w -X main.runMode=production -X main.version=$${VERSION}' myapp"
```

Now it works… but no one knows *why* those flags are there.
Your CI fails silently. Quoting breaks. Debugging becomes a guessing game.

---

## ✅ go-builder solves this, elegantly

Instead of spreading logic across bash, make, CI scripts, and tribal knowledge, `go-builder` lets you define your build process in a **clear, declarative, self-documented YAML file**:

```yaml
###############################################################################
# Build config for myapp — all in one place, documented and reproducible
###############################################################################

# Where to place the final binaries (default: "builds/")
build_dir: builds

# Entry point package and output name
source: ./cmd/myapp
output: myapp

# Global environment variables used by Go and CGO
env:
  CGO_ENABLED: "1"
  CC: gcc

# Compiler and linker settings
build:
  ldflags:
    - "-s -w" # Strip debug info and reduce binary size
    - "linkmode external -extldflags '-static'"  # Fully static binary
  vars:
    main.version: "${VERSION:-dev}"  # Embed build-time metadata or default to "dev"
    main.commit:  "${GIT_COMMIT:-unknown}" # Default to "unknown" if not set
  tags: ["prod"]
  trimpath: true # Remove file paths from binary
  debug: false

# Targets for cross-compilation
targets:
  - os: linux
    arch: amd64
  - os: linux
    arch: arm64

# Optional Docker container to perform the build
docker:
  image: golang:1.23-alpine                       # Use a consistent Go + musl toolchain
  setup:
    - apk add --no-cache build-base musl-dev      # Add build dependencies
    - git config --global url."https://${GIT_TOKEN}:x-oauth-basic@github.com/".insteadOf https://github.com/
  env:
    GIT_TERMINAL_PROMPT: "0"
    GOPRIVATE: github.com/acme_inc/*
```

📄 Every line is documented.
🧩 Every option is explainable.
🛠️ Every build is reproducible.

---

## 🔧 Install and use

```bash
go install github.com/pablolagos/go-builder@latest
```

Create a template config:

```bash
go-builder --init
```

Then run:

```bash
VERSION=1.3.0 go-builder
```

Or just preview the build without running it:

```bash
go-builder --dry-run
```

---

## 📚 Why this matters

With `go-builder`, your build process becomes:

* **Transparent** — no more guessing where flags come from
* **Portable** — Docker-based builds included
* **Maintainable** — easily edit or extend
* **Documented** — every line can carry its why

Whether you're building for Alpine, for `arm64`, or just want to *finally* understand your build pipeline —
`go-builder` brings clarity to your Go builds.

---

📦 Install it:

```bash
go install github.com/pablolagos/go-builder@latest
```

⚙️ Start a config file:

```bash
go-builder --init
```

🚀 Run your build:

```bash
go-builder
```

🧪 Preview without running:

```bash
go-builder --dry-run
```

---

With `go-builder`, your builds become readable, repeatable, and robust.
Just like your Go code.

---


Need a different file? Use `--config path/to/file.yml`.


## Features

| ✓                           | What it does                                                |
|-----------------------------|-------------------------------------------------------------|
| **Declarative YAML**        | (`.gobuilder.yml`) with full inline docs.                   |
| **Build matrix**            | generate binaries for any `GOOS/GOARCH` set.                |
| **Link-time vars**          | map → `-X 'name=value'`.                                    |
| **Placeholder expansion**   | `${VAR}` / `${VAR:-default}` everywhere.                    |
| **Per-target & global env** | `CC`, `CGO_LDFLAGS`, etc.                                   |
| **Auto build directory**    | (default `builds/`) & automatic `.gitignore` handling.      |
| **Dry-run**                 | (`--dry-run` or `build.debug:true`) to print commands only. |
| **Init template**           | (`--init`) writes a richly commented sample YAML.           |
| **No Git assumptions**      | use any branch, tag or detached HEAD.                       |
| **Docker builds**            | run builds in a Docker container with custom setup.         |
| **Cross-compilation**         | compile for any `GOOS/GOARCH` target.                        |



## Example `.gobuilder.yml`

```yaml
build_dir: builds
source: ./cmd/myapp
output: myapp-${VERSION:-dev}  # binary compiled name: myapp-dev if $VERSION is defined it will replace "dev". Usage of ${VAR:-default} is optional

env:
  CGO_ENABLED: "1"

build:
  ldflags: ["-s -w"]
  vars:
    main.version: "${VERSION:-dev}"
    main.commit:  "${COMMIT_SHA:-local}"
  tags: ["prod"]
  trimpath: true

targets:
  - os: linux
    arch: amd64
  - os: darwin
    arch: arm64
  - os: windows
    arch: amd64
```

Run `go-builder` and you’ll get:

```
builds/linux/amd64/myapp-dev
builds/darwin/arm64/myapp-dev
builds/windows/amd64/myapp-dev.exe
```

For example, to set a custom version:

```bash
VERSION=1.2.3 go-builder
```

This will produce binaries like:

```
builds/linux/amd64/myapp-1.2.3
builds/darwin/arm64/myapp-1.2.3
builds/windows/amd64/myapp-1.2.3.exe
```

All artifacts live under `build_dir`, already ignored by Git.

---

## CLI reference

| Flag            | Description                                         |
| --------------- | --------------------------------------------------- |
| `--config FILE` | Use FILE instead of `.gobuilder.yml`.               |
| `--dry-run, -n` | Print `go build …` commands but don’t execute.      |
| `--init, -i`    | Create `.gobuilder.yml` from the embedded template. |

---

## Contributing

Contributions are welcome!

## License

MIT © 2025 Pablo Lagos

