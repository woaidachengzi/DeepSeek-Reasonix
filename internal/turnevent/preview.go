package turnevent

import (
	"reasonix/internal/event"
	"reasonix/internal/eventwire"
)

func persistedWireEvent(e event.Event) eventwire.Event {
	w := eventwire.ToWire(e)
	// Screenshots are transient preview data. The original stamped event still
	// reaches live clients, while saved history and replay contain no image.
	if w.Tool != nil {
		w.Tool.ComputerUsePreview = nil
	}
	return w
}
