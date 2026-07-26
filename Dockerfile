# syntax=docker/dockerfile:1
# SPDX-License-Identifier: MIT

# sting as an MCP server, for docker-based MCP client configurations.
#
# This image is built by GoReleaser's dockers_v2 block from binaries it has
# already compiled for every target platform -- there is deliberately no build
# stage here. Compiling inside the image would produce a *different* binary from
# the one in the release archives, breaking the property that every channel is
# fed from one build: the same bytes that were signed, notarized, checksummed
# and attested are the bytes that ship here.
#
# The build context is laid out per platform by GoReleaser:
#   linux/amd64/sting
#   linux/arm64/sting

# Distroless static: no shell, no package manager, no libc to keep patched, and
# it already carries CA certificates for HTTPS to the provider APIs. Pinned by
# digest so a rebuild cannot silently pick up a different base.
FROM gcr.io/distroless/static@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6

# Set by buildx for each platform in the manifest.
ARG TARGETPLATFORM

COPY --chmod=0755 ${TARGETPLATFORM}/sting /usr/local/bin/sting

# uid/gid 65532 from the distroless nonroot variant. sting needs no privileges:
# it reads provider APIs over HTTPS and writes nothing outside its own config.
USER 65532:65532

# Credentials come from the environment at run time -- STING_TOKEN for GitHub,
# STING_GITLAB_TOKEN for GitLab. Nothing user- or organization-specific is baked
# into the image.

# With no arguments the container is an MCP server speaking stdio, which is what
# a docker-based MCP client configuration expects. Other subcommands remain
# reachable by overriding the command.
ENTRYPOINT ["/usr/local/bin/sting"]
CMD ["mcp"]
