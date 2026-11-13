package sqlstore

import (
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Small aliases keep tests readable without importing ports at every call site.
func portsFilter() ports.UserFilter         { return ports.UserFilter{} }
func portsMemoryFilter() ports.MemoryFilter { return ports.MemoryFilter{} }
