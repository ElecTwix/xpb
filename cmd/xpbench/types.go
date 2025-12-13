package main

// Result represents a single benchmark result.
type Result struct {
	Platform  string  // "Go", "Node", "Browser"
	Category  string  // "Small", "Large", "StringArray", "Int32Array", "StringMap"
	Format    string  // "XPB", "JSON", "Msgpack", "Protobuf"
	Operation string  // "Encode", "Decode"
	NsPerOp   float64 // Nanoseconds per operation
	Size      int64   // Size in bytes
}

// NodeResult matches the JSON structure from Node/Browser benchmarks.
type NodeResult struct {
	Name      string  `json:"name"`
	EncodeNs  float64 `json:"encodeNs"`
	DecodeNs  float64 `json:"decodeNs"`
	SizeBytes int64   `json:"sizeBytes"`
}

type NodeOutput struct {
	Small       []NodeResult `json:"small"`
	Large       []NodeResult `json:"large"`
	Collections struct {
		StringArray []NodeResult `json:"stringArray"`
		IntArray    []NodeResult `json:"intArray"`
		StringMap   []NodeResult `json:"stringMap"`
	} `json:"collections"`
}
