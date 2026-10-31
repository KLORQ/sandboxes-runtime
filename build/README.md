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

The gvproxy patch is experimental until matching integration and security
tests are recorded in the plan:

- `patches/gvproxy-egress-policy-v2.patch` — default-deny egress policy,
  revision reload, audit, flow revocation, and the supervisor owner lease.
  The daemon requires the patched `-require-owner-lease` option; older
  gvproxy builds are rejected at startup.

Nested Docker Engine remains blocked. The former nerdbox cgroup/capability
patch was removed because it applied broad capabilities and a writable bind of
the whole guest cgroup filesystem to every workload. Do not reintroduce it without an explicit
opt-in profile, least-privilege capability analysis, and guest-only cgroup
design validated against the pinned nerdbox build.

Patch SHA-256 values are recorded in `manifest.yaml` and checked by the
manifest test.

## Build outline

When producing the Linux guest EROFS image on macOS, specify a 4096-byte
block size explicitly. The macOS `mkfs.erofs` default can use 16384-byte
blocks, which the pinned guest kernel rejects during boot:

```sh
mkfs.erofs -b4096 -zlz4 /path/to/nerdbox-rootfs.erofs /path/to/guest-rootfs
```

The guest root filesystem must contain the vminitd built from the pinned
unmodified source. Do not use the removed broad cgroup/capability patch.

```sh
# gvproxy
git clone https://github.com/containers/gvisor-tap-vsock
cd gvisor-tap-vsock
git checkout 1cd3f0426b07b3a683d47c671edff8ca261225db
git apply /path/to/build/patches/gvproxy-egress-policy-v2.patch
go build -o gvproxy-policy-v2 ./cmd/gvproxy

```

The runtime adapter must consume these artifacts from explicit paths; it must
not download `@latest` components during a reproducible build.
