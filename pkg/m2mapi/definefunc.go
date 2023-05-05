package m2mapi

import "time"

type Area struct {
	//data property
	AreaID      string
	Address     string
	NE          SquarePoint
	SW          SquarePoint
	Description string
	// Name string
	// IoTSPIDs []string

	//object property
	//contains PSink
}

type PSink struct {
	//data property
	VPointID_n  string
	Address     string
	Lat         float64
	Lon         float64
	Description string
	VPointID    string //追加
	ServerIP    string //追加
	// Policy         string
	// IoTSPID        string
}

type ResolvePoint struct {
	//input
	NE SquarePoint
	SW SquarePoint

	//output
	VPointID_n string
	Address    string
}

type ResolveNode struct {
	//input
	VPointID_n string
	CapsInput  []string

	//output
	VNodeID_n string
	CapOutput string
}

type ResolvePastNode struct {
	//input
	VNodeID_n  string
	Capability string
	Period     PeriodInput

	//output
	//VNodeID_n 	string (dup)
	Values []Value
}

type ResolvePastPoint struct {
	//input
	VPointID_n string
	Capability string
	Period     PeriodInput

	//output
	Datas []SensorData
}

type ResolveCurrentNode struct {
	//input
	VNodeID_n  string
	Capability string

	//output
	//VNodeID_n 	string (dup)
	Values Value
}

type ResolveCurrentPoint struct {
	//input
	VPointID_n string
	Capability string

	//output
	Datas []SensorData
}

type ResolveConditionNode struct {
	//input
	VNodeID_n  string
	Capability string
	Limit      Range
	Timeout    time.Duration

	//output
	//DataForRegist
}

type SensorData struct {
	VNodeID_n string
	Values    []Value
}

type SquarePoint struct {
	Lat float64
	Lon float64
}

type PeriodInput struct {
	Start string
	End   string
}

type Value struct {
	Capability string
	Time       string
	Value      float64
}

type Range struct {
	LowerLimit float64
	UpperLimit float64
}

type DataForRegist struct {
	PNodeID    string
	Capability string
	Timestamp  string
	Value      string
	PSinkID    string
	ServerID   string
	Lat        string
	Lon        string
	VNodeID    string
	VPointID   string
}
