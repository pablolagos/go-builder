# go-builder

A **tiny, self-contained build helper for Go projects**.  
It turns a single YAML file into repeatable, documented `go build` commands—without forcing a particular Git workflow or release pipeline.

> **Why?**  
> We love <https://github.com/goreleaser/goreleaser>, but for some teams it feels heavy-weight: tight coupling to Git tags, opinionated release steps, lots of hidden defaults.  
> **go-builder** keeps only the compile step—**simple, explicit, zero magic**—so you can plug any packager (NFPM, `zig cc`, `tar`, `pkgbuild`, …) afterwards.

---

## Features

| ✓ | What it does |
|---|--------------|
| **Declarative YAML** (`.gobuilder.yml`) with full inline docs. |
| **Build matrix** – generate binaries for any `GOOS/GOARCH` set. |
| **Link-time vars** – map → `-X 'name=value'`. |
| **Placeholder expansion** – `${VAR}` / `${VAR:-default}` everywhere. |
| **Per-target & global env** – `CC`, `CGO_LDFLAGS`, etc. |
| **Auto build directory** (default `builds/`) & automatic `.gitignore` handling. |
| **Dry-run** (`--dry-run` or `build.debug:true`) to print commands only. |
| **Init template** (`--init`) writes a richly commented sample YAML. |
| **No Git assumptions** – use any branch, tag or detached HEAD. |

---

## Quick install

```bash
go install github.com/pablolagos/go-builder@latest
````

> Requires Go ≥ 1.22 because we use `go:embed`.

---

## Getting started in 30 sec

```bash
# 1. Drop a template in your repo root
go-builder --init      # creates .gobuilder.yml

# 2. Edit the file: adjust build matrix, ldflags, vars…

# 3. See what would run
go-builder --dry-run

# 4. Build all binaries
go-builder             # reads .gobuilder.yml by default
```

Need a different file? Use `--config path/to/file.yml`.

---

## Example `.gobuilder.yml`

```yaml
build_dir: builds
source: ./cmd/myapp
output: myapp-${VERSION:-dev}  # binary compiled name: myapp-dev if $VERSION is defined it will replace "dev"

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
  - os: linux   ; arch: amd64
  - os: darwin  ; arch: arm64
  - os: windows ; arch: amd64
```

Run `go-builder` and you’ll get:

```
builds/linux/amd64/myapp-<ver>
builds/darwin/arm64/myapp-<ver>
builds/windows/amd64/myapp-<ver>.exe
```

All artefacts live under `build_dir`, already ignored by Git.

---

## CLI reference

| Flag            | Description                                         |
| --------------- | --------------------------------------------------- |
| `--config FILE` | Use FILE instead of `.gobuilder.yml`.               |
| `--dry-run, -n` | Print `go build …` commands but don’t execute.      |
| `--init, -i`    | Create `.gobuilder.yml` from the embedded template. |

---

## Roadmap

* Parallel builds (`-j` style).
* Templating helpers for dates, semver, git info.
* Optional JSON summary for CI pipelines.
* **PRs welcome** – see `CONTRIBUTING.md` soon :)

---

## License

MIT © 2025 Pablo Lagos

```
