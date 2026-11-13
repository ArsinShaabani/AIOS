package sqlstore

// floatsToJSON converts a vector to a JSON-compatible slice. float32 marshals as
// a number with a short representation, keeping embeddings compact on disk.
func floatsToJSON(v []float32) []float32 {
	if v == nil {
		return []float32{}
	}
	return v
}
