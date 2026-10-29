# Runtime build pins

This directory pins the native runtime inputs used by the containerd/nerdbox
adapter. The manifest is authoritative; the patches are checked by the
`internal/manifest` test.

## Components

- containerd `2.4.2`
- nerdbox `0.2.5`, commit `28c86e8e16c62a08079531ebe99e24a7bdad3d62`
- libkrun `1.19.6`
- gvisor-tap-vsock commit `1cd3f0426b07b3a683d47c671edff8ca261225db`
- default image `docker.io/library/alpine:3.22` at the pinned digest

## Patches

Both patches are experimental until the matching integration and security
tests are recorded in the plan:

- `patches/gvproxy-egress-policy-v2.patch` — default-deny egress policy,
  revision reload, audit, and flow revocation.
- `patches/nerdbox-klorq-cgroup-caps.patch` — guest workload cgroup and
  capability preparation for a private Docker Engine.

Patch SHA-256 values are recorded in `manifest.yaml` and checked by the
manifest test.

## Build outline

```sh
# gvproxy
git clone https://github.com/containers/gvisor-tap-vsock
cd gvisor-tap-vsock
git checkout 1cd3f0426b07b3a683d47c671edff8ca261225db
git apply /path/to/build/patches/gvproxy-egress-policy-v2.patch
go build -o gvproxy-policy-v2 ./cmd/gvproxy

# nerdbox
git clone https://github.com/containerd/nerdbox
cd nerdbox
git checkout 28c86e8e16c62a08079531ebe99e24a7bdad3d62
git apply /path/to/build/patches/nerdbox-klorq-cgroup-caps.patch
# build the nerdbox shim and kernel/rootfs bundle using the upstream Makefile
```

The runtime adapter must consume these artifacts from explicit paths; it must
not download `@latest` components during a reproducible build.
