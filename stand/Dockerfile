FROM debian:trixie-slim

# Build toolchain for xswitcher (cgo + X11 bindings) and the stand's observation tools.
RUN apt-get update -qq && apt-get install -y --no-install-recommends \
    ca-certificates golang-go gcc pkg-config libx11-dev libxmu-dev \
    xvfb x11-utils x11-xkb-utils xkb-data xdotool python3 python3-evdev kbd \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /w
CMD ["bash", "/w/stand/run.sh"]
