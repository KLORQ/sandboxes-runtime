// Package runtime defines the VM backend surface.
package runtime

import (
	"context"
	"time"
)

// Capabilities describes backend support.
type Capabilities struct {
	Workspace bool
	Network   bool
	Snapshots bool
	GPU       bool
	IPv6      bool
}

// Config describes a VM create request.
type Config struct {
	Name      string
	Image     string
	Workspace string
	CPUs      uint8
	MemoryMiB uint32
	FIFODir   string
}

// Handle identifies a runtime instance.
type Handle struct {
	ID      string
	FIFODir string
}

// NetworkAttachment is the host network data required by the runtime.
type NetworkAttachment struct {
	SocketPath  string
	MAC         string
	DeviceIP    string
	GatewayIP   string
	Annotations []string
}

// Network starts and stops the host-side gateway for a sandbox.
type Network interface {
	Start(ctx context.Context, sandbox string) (NetworkAttachment, error)
	Stop(ctx context.Context, sandbox string) error
}

// Backend is implemented by the containerd/nerdbox adapter.
type Backend interface {
	Capabilities(ctx context.Context) (Capabilities, error)
	Create(ctx context.Context, cfg Config) (Handle, error)
	Start(ctx context.Context, h Handle) error
	Stop(ctx context.Context, h Handle, grace time.Duration) error
	Delete(ctx context.Context, h Handle) error
	Handle(name string) Handle
}
