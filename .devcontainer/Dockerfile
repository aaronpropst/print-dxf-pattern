FROM mcr.microsoft.com/devcontainers/python:2-3.13-trixie

# Remove the broken Yarn apt source (missing GPG key causes apt-get update to fail)
RUN rm -f /etc/apt/sources.list.d/yarn.list \
          /etc/apt/sources.list.d/yarnpkg.list \
          /etc/apt/trusted.gpg.d/yarn*.gpg \
    2>/dev/null || true

# Install Go (latest stable, architecture-aware)
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates && \
    LATEST=$(curl -fsSL "https://go.dev/VERSION?m=text" | head -1) && \
    ARCH=$(dpkg --print-architecture) && \
    curl -fsSL "https://go.dev/dl/${LATEST}.linux-${ARCH}.tar.gz" | tar -C /usr/local -xz && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

ENV PATH="/usr/local/go/bin:/go/bin:${PATH}"
ENV GOPATH="/go"
