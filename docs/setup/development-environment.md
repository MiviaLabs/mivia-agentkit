# Development Environment

This repo uses committed Git hooks plus a Makefile-driven local quality gate. Install the tools below before running `make install-hooks` or `make verify`.

## Required Tools

- Git
- Bash
- GNU Make
- Python 3.10 or newer
- Semgrep CLI
- Go, once Go files exist; `go.mod` will become the canonical version source when it lands

Current local verification was run with:

- Go `1.26.0`
- Python `3.14.4`
- Semgrep `1.167.0`
- GNU Make `4.4.1`
- Git `2.53.0`
- Bash `5.3.9`

## Ubuntu Install

Base tools and Python:

```bash
sudo apt update
sudo apt install -y git bash make python3 python3-venv python3-pip pipx
pipx ensurepath
pipx install semgrep
```

Install Go with one of these paths:

```bash
# Ubuntu package path
sudo apt update
sudo apt install -y golang-go
```

```bash
# Snap path for current stable Go
sudo snap install go --classic
```

If you need a specific Go version, use the official Go tarball install flow from `go.dev/dl/` and add `/usr/local/go/bin` to `PATH`.

## Verify

```bash
git --version
bash --version
make --version
python3 --version
semgrep --version
go version
```

Restart the shell if `semgrep` or `go` is not found after installation.

Then run:

```bash
make install-hooks
make verify
```

## Sources

- Go install docs: https://go.dev/doc/install
- Go on Ubuntu: https://go.dev/wiki/Ubuntu and https://ubuntu.com/developers/docs/howto/go-setup/
- Semgrep install docs: https://docs.semgrep.dev/getting-started/quickstart
- pipx install docs: https://pipx.pypa.io/stable/how-to/install-pipx/
- Python virtual environment docs: https://docs.python.org/3/library/venv.html
