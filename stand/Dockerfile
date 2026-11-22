# golang:1.26 rather than debian:trixie-slim + golang-go: the module pins go 1.26.0 / toolchain
# go1.26.11 (the govulncheck gate goes red on any older patch release -- .github/workflows/ci.yml
# carries the measurement next to the pin), and the Go shipped in the Debian archive is older than that.
FROM golang:1.26-trixie

# Build toolchain for xswitcher and the stand's observation tools -- from the same file the GUI image
# and .github/workflows/ci.yml read, so a header cannot be present in one environment and missing in
# another. It used to carry its own shorter list, and that is what broke: `go test ./...` (check B3 of
# stand/run.sh) compiles cmd/gui, Fyne pulls glfw, and glfw's glfw3native.h includes
# <X11/extensions/Xrandr.h> -- absent here, present in stand/apt-deps.txt. Measured: this image answered
# "ls: cannot access '/usr/include/X11/extensions/Xrandr.h': No such file or directory" while
# xswitcher-gui-stand had the file. `tr -d '\r'` matters for the same reason as in Dockerfile.gui: a
# CRLF checkout makes xargs ask apt for "xvfb\r".
COPY stand/apt-deps.txt /tmp/apt-deps.txt

RUN apt-get update \
    && tr -d '\r' < /tmp/apt-deps.txt | grep -v '^#' | xargs -r apt-get install -y --no-install-recommends \
    && rm -rf /var/lib/apt/lists/* /tmp/apt-deps.txt

WORKDIR /w
CMD ["bash", "/w/stand/run.sh"]
