package ctr

import (
	"context"
	"fmt"
	"time"

	runtime "github.com/KLORQ/sandboxes-runtime"
)

// BackendOptions configures the runtime backend.
type BackendOptions struct {
	Image   string
	FIFODir string
}

// Backend implements the runtime.Backend contract with ctr.
type Backend struct {
	client  *Client
	image   string
	fifoDir string
}

var _ runtime.Backend = (*Backend)(nil)

// NewBackend creates a containerd-backed runtime.
func NewBackend(cfg Config, opts BackendOptions) (*Backend, error) {
	client, err := New(cfg)
	if err != nil {
		return nil, err
	}
	return &Backend{client: client, image: opts.Image, fifoDir: opts.FIFODir}, nil
}

// Capabilities reports the current adapter capabilities.
func (b *Backend) Capabilities(context.Context) (runtime.Capabilities, error) {
	return runtime.Capabilities{Workspace: true, Network: false, Snapshots: false, GPU: false, IPv6: false}, nil
}

// Create creates and starts a detached nerdbox task.
func (b *Backend) Create(ctx context.Context, cfg runtime.Config) (runtime.Handle, error) {
	image := cfg.Image
	if image == "" {
		image = b.image
	}
	fifoDir := cfg.FIFODir
	if fifoDir == "" {
		fifoDir = b.fifoDir
	}
	if err := b.client.EnsureImage(ctx, image); err != nil {
		return runtime.Handle{}, err
	}
	id := ContainerID(cfg.Name)
	if b.client.ContainerExists(ctx, id) {
		return runtime.Handle{}, fmt.Errorf("container %s already exists", id)
	}
	spec := RunSpec{
		ContainerID: id,
		Name:        cfg.Name,
		Image:       image,
		CPUs:        cfg.CPUs,
		MemoryMiB:   cfg.MemoryMiB,
		FIFODir:     fifoDir,
	}
	if cfg.Workspace != "" {
		spec.Mounts = append(spec.Mounts, Mount{Source: cfg.Workspace, Destination: cfg.Workspace})
	}
	if err := b.client.Run(ctx, spec); err != nil {
		return runtime.Handle{}, err
	}
	return runtime.Handle{ID: id, FIFODir: fifoDir}, nil
}

// Handle returns the runtime handle for a sandbox name.
func (b *Backend) Handle(name string) runtime.Handle {
	return runtime.Handle{ID: ContainerID(name), FIFODir: b.fifoDir}
}

// Start starts a stopped task.
func (b *Backend) Start(ctx context.Context, h runtime.Handle) error {
	return b.client.StartTask(ctx, h.ID, h.FIFODir)
}

// Stop terminates a task.
func (b *Backend) Stop(ctx context.Context, h runtime.Handle, grace time.Duration) error {
	if grace <= 0 {
		grace = 5 * time.Second
	}
	return b.client.StopTask(ctx, h.ID, grace)
}

// Delete removes the task and container.
func (b *Backend) Delete(ctx context.Context, h runtime.Handle) error {
	_ = b.client.DeleteTask(ctx, h.ID)
	return b.client.DeleteContainer(ctx, h.ID)
}

// ContainerID returns the containerd container id for a sandbox name.
func ContainerID(name string) string { return "klorq-" + name }
