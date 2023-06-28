package main

import (
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"time"

	"mecm2m-Simulator/pkg/m2mapi"
	"mecm2m-Simulator/pkg/m2mapp"
	"mecm2m-Simulator/pkg/message"
)

const (
	protocol                  = "unix"
	m2mapi_server_socket_file = "/tmp/mecm2m/svr_1_m2mapi.sock"
)

type Format struct {
	FormType string
}

func main() {
	// コマンドライン引数に App の入力内容をまとめたファイルを指定して，初めにそのファイルを読み込む
	// App名を指定して，Appごとに実行する内容を分岐
	if len(os.Args) != 2 {
		fmt.Println("There is no input file")
		os.Exit(1)
	}

	app_input_data := os.Args[1]
	data, err := ioutil.ReadFile(app_input_data)
	if err != nil {
		message.MyError(err, "Failed to read input data file")
	}

	var input_data m2mapp.AppInputData

	if err := json.Unmarshal(data, &input_data); err != nil {
		message.MyError(err, "Failed to unmarshal json")
	}

	// App名ごとに，実行する内容を分岐
	switch input_data.AppName {
	case "FrozenRoad":
		output_data := frozenRoad(input_data)
		fmt.Println(output_data)
	}
}

func frozenRoad(input_data m2mapp.AppInputData) m2mapi.DataForRegist {
	// ポイント解決
	command := "point"
	connRP, err := net.Dial(protocol, m2mapi_server_socket_file)
	if err != nil {
		message.MyError(err, "frozenRoad > connRP > net.Dial")
	}

	decoderRP := gob.NewDecoder(connRP)
	encoderRP := gob.NewEncoder(connRP)
	// M2M API 実行前の同期
	syncFormatClient(command, decoderRP, encoderRP)
	// M2M API実行
	point_output := m2mAPIPoint(decoderRP, encoderRP, input_data)
	connRP.Close()

	// ノード解決
	command = "node"
	connRN, err := net.Dial(protocol, m2mapi_server_socket_file)
	if err != nil {
		message.MyError(err, "frozenRoad > connRN > net.Dial")
	}

	decoderRN := gob.NewDecoder(connRN)
	encoderRN := gob.NewEncoder(connRN)
	// M2M API 実行前の同期
	syncFormatClient(command, decoderRN, encoderRN)
	// M2M API実行
	node_output := m2mAPINode(decoderRN, encoderRN, point_output, input_data.Capability)
	connRN.Close()

	// ノード指定型充足条件データ取得
	command = "condition_node"
	connConN, err := net.Dial(protocol, m2mapi_server_socket_file)
	if err != nil {
		message.MyError(err, "frozenRoad > connConN > net.Dial")
	}

	decoderConN := gob.NewDecoder(connConN)
	encoderConN := gob.NewEncoder(connConN)
	// M2M API 実行前の同期
	syncFormatClient(command, decoderConN, encoderConN)
	// M2M API実行
	condition_node_output := m2mAPIConditionNode(decoderConN, encoderConN, node_output, input_data.Limit, input_data.Timeout)

	// アクチュエータ実行
	command = "actuate"
	connAct, err := net.Dial(protocol, m2mapi_server_socket_file)
	if err != nil {
		message.MyError(err, "frozenRoad > connAct > net.Dial")
	}

	decoderAct := gob.NewDecoder(connAct)
	encoderAct := gob.NewEncoder(connAct)
	// M2M API 実行前の同期
	syncFormatClient(command, decoderAct, encoderAct)
	// M2M API 実行
	actuate_output := m2mAPIActuate(decoderAct, encoderAct, node_output, input_data.Action, input_data.Parameter)
	fmt.Println(actuate_output)

	return condition_node_output
}

func m2mAPIPoint(decoderRP *gob.Decoder, encoderRP *gob.Encoder, input_data m2mapp.AppInputData) []m2mapi.ResolvePoint {
	var swlat, swlon, nelat, nelon float64
	swlat = input_data.SW.Lat
	swlon = input_data.SW.Lon
	nelat = input_data.NE.Lat
	nelon = input_data.NE.Lon
	point_input := &m2mapi.ResolvePoint{
		SW: m2mapi.SquarePoint{Lat: swlat, Lon: swlon},
		NE: m2mapi.SquarePoint{Lat: nelat, Lon: nelon},
	}
	if err := encoderRP.Encode(point_input); err != nil {
		message.MyError(err, "m2mAPIPoint > encoderRP.Encode")
	}

	point_output := []m2mapi.ResolvePoint{}
	if err := decoderRP.Decode(&point_output); err != nil {
		message.MyError(err, "m2mAPIPoint > decoderRP.Decode")
	}
	return point_output
}

func m2mAPINode(decoderRN *gob.Decoder, encoderRN *gob.Encoder, vpointid []m2mapi.ResolvePoint, capability []string) []m2mapi.ResolveNode {
	VPointID_n := vpointid[0].VPointID_n
	Caps := capability
	node_input := &m2mapi.ResolveNode{
		VPointID_n: VPointID_n,
		CapsInput:  Caps,
	}
	if err := encoderRN.Encode(node_input); err != nil {
		message.MyError(err, "m2mAPINode > encoderRN.Encode")
	}

	node_output := []m2mapi.ResolveNode{}
	if err := decoderRN.Decode(&node_output); err != nil {
		message.MyError(err, "m2mAPINode > decoderRN.Decode")
	}
	return node_output
}

func m2mAPIConditionNode(decoderConN *gob.Decoder, encoderConN *gob.Encoder, node_output []m2mapi.ResolveNode, limit m2mapp.Range, timeout int) m2mapi.DataForRegist {
	VNodeID_n := node_output[0].VNodeID_n
	Capability := node_output[0].CapOutput
	LowerLimit := limit.LowerLimit
	UpperLimit := limit.UpperLimit
	Timeout := time.Duration(timeout * int(time.Second))
	condition_node_input := &m2mapi.ResolveConditionNode{
		VNodeID_n:  VNodeID_n,
		Capability: Capability,
		Limit:      m2mapi.Range{LowerLimit: LowerLimit, UpperLimit: UpperLimit},
		Timeout:    time.Duration(Timeout),
	}
	if err := encoderConN.Encode(condition_node_input); err != nil {
		message.MyError(err, "m2mAPIConditionNode > encoderConN.Encode")
	}

	condition_node_output := m2mapi.DataForRegist{}
	if err := decoderConN.Decode(&condition_node_output); err != nil {
		message.MyError(err, "m2mAPIConditionNode > decoderConN.Decode")
	}
	return condition_node_output
}

func m2mAPIActuate(decoderAct *gob.Decoder, encoderAct *gob.Encoder, node_output []m2mapi.ResolveNode, action string, parameter float64) m2mapi.Actuate {
	VNodeID_n := node_output[0].VNodeID_n
	Action := action
	Parameter := parameter
	actuate_input := &m2mapi.Actuate{
		VNodeID_n: VNodeID_n,
		Action:    Action,
		Parameter: Parameter,
	}
	if err := encoderAct.Encode(actuate_input); err != nil {
		message.MyError(err, "m2mAPIActuate > encoderAct.Encode")
	}

	actuate_output := m2mapi.Actuate{}
	if err := decoderAct.Decode(&actuate_output); err != nil {
		message.MyError(err, "m2mAPIActuate > decoderAct.Decode")
	}
	return actuate_output
}

func syncFormatClient(command string, decoder *gob.Decoder, encoder *gob.Encoder) {
	format := &Format{}
	switch command {
	case "point":
		format.FormType = "Point"
	case "node":
		format.FormType = "Node"
	case "past_node":
		format.FormType = "PastNode"
	case "past_point":
		format.FormType = "PastPoint"
	case "current_node":
		format.FormType = "CurrentNode"
	case "current_point":
		format.FormType = "CurrentPoint"
	case "condition_node":
		format.FormType = "ConditionNode"
	case "condition_point":
		format.FormType = "ConditionPoint"
	}
	if err := encoder.Encode(format); err != nil {
		message.MyError(err, "syncFormatClient > "+command+" > encoder.Encode")
	}
}
