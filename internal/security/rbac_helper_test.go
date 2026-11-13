package security

// PermAnything returns a permission no role necessarily holds; it proves the
// admin wildcard in the RBAC matrix.
func PermAnything() Permission { return Permission("nonexistent:permission") }
