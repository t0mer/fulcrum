package sink

import (
	"context"
	"fmt"
)

// ImageSender is the slice of the WhatsApp provider the forward sink needs.
type ImageSender interface {
	SendImage(ctx context.Context, groupID string, img []byte, mime, caption string) error
}

// Forward sends matched images to a destination WhatsApp group.
type Forward struct {
	Sender             ImageSender
	DestinationGroupID string
}

// Send forwards one image. It is a no-op (nil) when no destination is set.
func (f *Forward) Send(ctx context.Context, img []byte, mime, caption string) error {
	if f.DestinationGroupID == "" {
		return nil
	}
	if err := f.Sender.SendImage(ctx, f.DestinationGroupID, img, mime, caption); err != nil {
		return fmt.Errorf("forwarding to %s: %w", f.DestinationGroupID, err)
	}
	return nil
}
