package ctr

import (
	"context"
	"fmt"
	"strings"
	"time"

	runtime "github.com/KLORQ/sandboxes-runtime"
)

// BackendOptions configures the runtime backend.
type BackendOptions struct {
	Image   string
	FIFODir string
	Network runtime.Network
}

// Backend implements the runtime.Backend contract with ctr.
type Backend struct {
	client  *Client
	image   string
	fifoDir string
	network runtime.Network
}

var _ runtime.Backend = (*Backend)(nil)

// NewBackend creates a containerd-backed runtime.
func NewBackend(cfg Config, opts BackendOptions) (*Backend, error) {
	client, err := New(cfg)
	if err != nil {
		return nil, err
	}
	return &Backend{client: client, image: opts.Image, fifoDir: opts.FIFODir, network: opts.Network}, nil
}

// Capabilities reports the current adapter capabilities.
func (b *Backend) Capabilities(context.Context) (runtime.Capabilities, error) {
	return runtime.Capabilities{
		Workspace: true,
		Network:   b.network != nil,
		Snapshots: false,
		GPU:       false,
		IPv6:      false,
	}, nil
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
	var attachment runtime.NetworkAttachment
	if b.network != nil {
		var err error
		attachment, err = b.network.Start(ctx, cfg.Name)
		if err != nil {
			return runtime.Handle{}, err
		}
	}
	spec := RunSpec{
		ContainerID: id,
		Name:        cfg.Name,
		Image:       image,
		CPUs:        cfg.CPUs,
		MemoryMiB:   cfg.MemoryMiB,
		FIFODir:     fifoDir,
		Annotations: append([]string(nil), attachment.Annotations...),
	}
	if cfg.Workspace != "" {
		spec.Mounts = append(spec.Mounts, Mount{Source: cfg.Workspace, Destination: cfg.Workspace})
	}
	if err := b.client.Run(ctx, spec); err != nil {
		if b.network != nil {
			_ = b.network.Stop(ctx, cfg.Name)
		}
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
	if b.network != nil {
		if _, err := b.network.Start(ctx, strings.TrimPrefix(h.ID, "klorq-")); err != nil {
			return err
		}
	}
	if err := b.client.StartTask(ctx, h.ID, h.FIFODir); err != nil {
		if b.network != nil {
			_ = b.network.Stop(ctx, strings.TrimPrefix(h.ID, "klorq-"))
		}
		return err
	}
	return nil
}

// Stop terminates a task.
func (b *Backend) Stop(ctx context.Context, h runtime.Handle, grace time.Duration) error {
	if grace <= 0 {
		grace = 5 * time.Second
	}
	err := b.client.StopTask(ctx, h.ID, grace)
	if b.network != nil {
		if stopErr := b.network.Stop(ctx, strings.TrimPrefix(h.ID, "klorq-")); err == nil {
			err = stopErr
		}
	}
	return err
}

// Delete removes the task and container.
func (b *Backend) Delete(ctx context.Context, h runtime.Handle) error {
	err := b.client.DeleteTask(ctx, h.ID)
	if err == nil {
		err = b.client.DeleteContainer(ctx, h.ID)
	}
	if b.network != nil {
		if stopErr := b.network.Stop(ctx, strings.TrimPrefix(h.ID, "klorq-")); err == nil {
			err = stopErr
		}
	}
	return err
}

// ContainerID returns the containerd container id for a sandbox name.
func ContainerID(name string) string { return "klorq-" + name }
