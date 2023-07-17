package m2mapi

import "time"

type Area struct {
	// data property
	AreaID      string
	Address     string
	NE          SquarePoint
	SW          SquarePoint
	Description string
	// Name string
	// IoTSPIDs []string

	// object property
	//contains PSink
}

type PSink struct {
	// data property
	VPointID_n  string
	Address     string
	Lat         float64
	Lon         float64
	Description string
	VPointID    string //追加
	ServerIP    string //追加
	//Policy         string
	//IoTSPID        string
}

type ResolvePoint struct {
	// input
	NE SquarePoint
	SW SquarePoint

	// output
	VPointID_n    string
	SocketAddress string

	// etc.
	DestSocketAddr string // リンクプロセスが宛先のソケットアドレスを認識するために必要
}

type ResolveNode struct {
	// input
	VPointID_n string
	CapsInput  []string

	// output
	VNodeID_n     string
	CapOutput     string
	SocketAddress string

	// etc.
	DestSocketAddr string // リンクプロセスが宛先のソケットアドレスを認識するために必要
}

type ResolvePastNode struct {
	// input
	VNodeID_n     string
	Capability    string
	Period        PeriodInput
	SocketAddress string // センサデータ取得対象となるVNodeのソケットアドレス

	// output
	//VNodeID_n 	string (dup)
	Values []Value

	// etc.
	DestSocketAddr string // リンクプロセスが宛先のソケットアドレスを認識するために必要
}

type ResolvePastPoint struct {
	// input
	VPointID_n    string
	Capability    string
	Period        PeriodInput
	SocketAddress string // センサデータ取得対象となるVPointのソケットアドレス

	// output
	Datas []SensorData

	// etc.
	DestSocketAddr string // リンクプロセスが宛先のソケットアドレスを認識するために必要
}

type ResolveCurrentNode struct {
	// input
	VNodeID_n     string
	Capability    string
	SocketAddress string // センサデータ取得対象となるVNodeのソケットアドレス

	// output
	//VNodeID_n 	string (dup)
	Values Value

	// etc.
	DestSocketAddr string // リンクプロセスが宛先のソケットアドレスを認識するために必要
}

type ResolveCurrentPoint struct {
	// input
	VPointID_n    string
	Capability    string
	SocketAddress string // センサデータ取得対象となるVPointのソケットアドレス

	// output
	Datas []SensorData

	// etc.
	DestSocketAddr string // リンクプロセスが宛先のソケットアドレスを認識するために必要
}

type ResolveConditionNode struct {
	// input
	VNodeID_n     string
	Capability    string
	Limit         Range
	Timeout       time.Duration
	SocketAddress string // センサデータ取得対象となるVNodeのソケットアドレス

	// output
	//DataForRegist

	// etc.
	DestSocketAddr string // リンクプロセスが宛先のソケットアドレスを認識するために必要
}

type ResolveConditionPoint struct {
	// input
	VPointID_n    string
	Capability    string
	Limit         Range
	Timeout       time.Duration
	SocketAddress string // センサデータ取得対象となるVPointのソケットアドレス

	// etc.
	DestSocketAddr string // リンクプロセスが宛先のソケットアドレスを認識するために必要
}

type Actuate struct {
	// input
	VNodeID_n     string
	Action        string
	Parameter     float64
	SocketAddress string // 動作指示対象となるVNodeのソケットアドレス

	// output
	Status bool

	// etc.
	DestSocketAddr string // リンクプロセスが宛先のソケットアドレスを認識するために必要
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
	Value      float64
	PSinkID    string
	Lat        float64
	Lon        float64
}
