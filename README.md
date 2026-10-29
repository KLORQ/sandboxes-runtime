# sandboxes-runtime

Status: pre-alpha.

Runtime adapters for containerd/nerdbox, VM lifecycle, workspace mounts, OCI
images, templates, and kits. Implements ports defined by sandboxes-core.

## Boundaries

- does not expose daemon or SDK types;
- does not make network policy decisions;
- host Docker socket is never mounted.

## Runtime pins

`build/manifest.yaml` pins containerd, nerdbox, libkrun, gvisor-tap-vsock,
patches, and the default image digest. Validate the checked-in inputs with:

```sh
make runtime-verify
```

## containerd adapter

`ctr` is the first lifecycle adapter. It uses a pinned containerd CLI and
implements the `runtime.Backend` contract: image pull, detached run, task
start/stop/delete, and workspace bind mounts.

Current limits are explicit:

- `Capabilities.Network` is `false`; gateway wiring is the next step;
- no exec/PTY yet;
- no snapshots or GPU;
- adapter is covered by command-construction tests, not a live VM test yet.
