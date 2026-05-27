# OpenGL EGL render verification for the Go binding.
#
# Builds the upstream native library for the linux-{arm64,x64}-egl variant,
# builds the go-readback example, and runs it under a software OpenGL stack
# (Mesa llvmpipe + EGL surfaceless). Produces /out/go-readback.ppm so the
# caller can inspect the rendered frame.
#
# Build:
#   docker build --platform=linux/arm64 \
#     -t maplibre-go-readback \
#     -f examples/go-readback/Dockerfile .
# Run:
#   docker run --rm -v "$PWD/out:/out" maplibre-go-readback
#
# linux/arm64 runs natively on Apple Silicon; switch to linux/amd64 if you're
# on an x64 host (the EGL variant config is selected from the host arch via
# the EGL_VARIANT build arg).

FROM ubuntu:24.04

ARG EGL_VARIANT=linux-arm64-egl

ENV DEBIAN_FRONTEND=noninteractive

# Toolchain prerequisites for mise + pixi + cmake build, plus the Mesa
# software EGL/GL stack the example needs at runtime.
RUN apt-get update && apt-get install --no-install-recommends --yes \
    ca-certificates curl xz-utils git build-essential \
    pkg-config python3 \
    libegl-mesa0 libgl1-mesa-dri mesa-utils libegl1 libegl-dev \
 && rm -rf /var/lib/apt/lists/*

# Install mise (toolchain manager used by upstream).
RUN curl https://mise.run | sh \
 && ln -s /root/.local/bin/mise /usr/local/bin/mise

ENV PATH="/root/.local/bin:/root/.local/share/mise/shims:${PATH}" \
    MISE_ENV=${EGL_VARIANT}

# Software OpenGL knobs — same values CI sets via setup-ci-deps.
ENV EGL_PLATFORM=surfaceless \
    LIBGL_ALWAYS_SOFTWARE=true

# Constrain compile parallelism. Each C++ unit can use ~1.5 GB; the default
# Docker Desktop memory pool (≈8 GB) OOM-kills cc1plus when ninja schedules
# all cores at once. Override with --build-arg PARALLEL=N if your host has
# more memory available.
ARG PARALLEL=3
ENV CMAKE_BUILD_PARALLEL_LEVEL=${PARALLEL}

WORKDIR /work

# Copy the repo. We need .gitmodules + the third_party submodule sources for
# the upstream native build, so the caller should run this with the submodules
# checked out (mise install pulls everything else, including the cmake config).
COPY . /work

# mise reads .mise/config.${MISE_ENV}.toml automatically when MISE_ENV is set.
RUN mise trust . && mise install

# Build the upstream native library for the EGL variant.
RUN mise run configure && mise run build

# Build the Go example. The mise task wires CGO_CFLAGS / CGO_LDFLAGS using the
# variant's MLN_FFI_BUILD_DIR.
RUN mise run //examples/go-readback:build

# Verify EGL is wired up before the entrypoint runs the render.
RUN eglinfo | head -20 || true

VOLUME ["/out"]

CMD ["bash", "-c", "mise run //examples/go-readback:run && cp examples/go-readback/out/go-readback.ppm /out/go-readback.ppm && ls -la /out/"]
