package controller

// Event represents a typed event.
type Event struct {
	// Type is the type of event.
	Type EventType
	// Object is the event subject.
	Object interface{}
	// OldObject is the old object in update event.
	OldObject interface{}
	// Tombstone is the final state before object was delete,
	// it's useful for DELETE event.
	Tombstone interface{}
}

// EventType is the type of event.
type EventType int

const (
	// EventAdd means an add event.
	EventAdd = iota + 1
	// EventUpdate means an update event.
	EventUpdate
	// EventDelete means a delete event.
	EventDelete
)

func (ev EventType) String() string {
	switch ev {
	case EventAdd:
		return "add"
	case EventUpdate:
		return "update"
	case EventDelete:
		return "delete"
	default:
		return "unknown"
	}
}
