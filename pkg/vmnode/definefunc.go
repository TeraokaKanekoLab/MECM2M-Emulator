package vmnode

type ResolveDataByNode struct {
	// input
	// Local SensingDB へのクエリ

	// output
	PNodeID    string  `json:"pnode-id"`
	Capability string  `json:"capability"`
	Timestamp  string  `json:"timestamp"`
	Value      float64 `json:"value"`
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
}
