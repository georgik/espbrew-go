# syntax=docker/dockerfile:1
#
# ---------------------------------------------------------------------------
# ESPBrew cluster image
#
# This image bundles the PRE-BUILT espbrew release binary. It does NOT compile
# anything inside the image — it simply downloads the release asset
# (espbrew-linux-amd64) and starts it in cluster/leader mode, mirroring
# cluster.sh:
#
#     ./espbrew cluster --role leader --port 8080
#
# To bundle a different release version, rebuild with:
#     docker build --build-arg ESPBREW_VERSION=v0.4.0 -t ghcr.io/<owner>/<repo> .
#
# Base image note: the release binary is dynamically linked against glibc
# (interpreter /lib64/ld-linux-x86-64.so.2, NEEDED libc.so.6), so the base
# image MUST provide a glibc runtime. debian:slim does; alpine/musl would NOT
# run the binary.
# ---------------------------------------------------------------------------

# --- release asset to bundle ------------------------------------------------
ARG ESPBREW_VERSION=v0.4.0
ARG BINARY_NAME=espbrew-linux-amd64        # linux/amd64 asset (see release.yml)
ARG RELEASE_URL=https://github.com/georgik/espbrew-go/releases/download

# --- runtime image (glibc) --------------------------------------------------
FROM debian:bookworm-slim

# ca-certificates (TLS to peers) + curl (used by the HEALTHCHECK below).
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

ARG ESPBREW_VERSION
ARG BINARY_NAME
ARG RELEASE_URL

# Download the pre-built release binary (no build step inside the image).
RUN curl -fSL --retry 3 -o /usr/local/bin/espbrew \
        "${RELEASE_URL}/${ESPBREW_VERSION}/${BINARY_NAME}" \
    && chmod +x /usr/local/bin/espbrew \
    && echo "Bundled espbrew version: ${ESPBREW_VERSION}"

# The cluster dashboard + API listen here.
EXPOSE 8080

# Liveness probe against the dashboard root.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -fSs http://localhost:8080/ || exit 1

# Start espbrew as a cluster leader, exactly like cluster.sh.
ENTRYPOINT ["/usr/local/bin/espbrew"]
CMD ["cluster", "--role", "leader", "--port", "8080"]
