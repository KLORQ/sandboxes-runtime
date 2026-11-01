package ctr

import (
	"context"
	"fmt"

	containerd "github.com/containerd/containerd/v2/client"
)

// ImageLabels returns OCI config labels for a locally available image.
func (b *Backend) ImageLabels(ctx context.Context, image string) (map[string]string, error) {
	client, err := containerd.New(b.client.cfg.Socket, containerd.WithDefaultNamespace(b.client.cfg.Namespace))
	if err != nil {
		return nil, fmt.Errorf("connect containerd: %w", err)
	}
	defer func() { _ = client.Close() }()
	img, err := client.GetImage(ctx, image)
	if err != nil {
		return nil, fmt.Errorf("get image: %w", err)
	}
	spec, err := img.Spec(ctx)
	if err != nil {
		return nil, fmt.Errorf("read image spec: %w", err)
	}
	return spec.Config.Labels, nil
}
