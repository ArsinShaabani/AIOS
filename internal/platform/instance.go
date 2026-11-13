package platform

// instanceID identifies this process on a shared event bus so a replica can skip
// events it produced itself.
var instanceID = NewID("inst")

// InstanceID returns the process identifier.
func InstanceID() string { return instanceID }
