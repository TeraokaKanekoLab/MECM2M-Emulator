package main

import (
	"encoding/csv"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"mecm2m-Simulator/pkg/m2mapi"
	"mecm2m-Simulator/pkg/message"

	"github.com/joho/godotenv"
)

const (
	protocol = "unix"
	timeSock = "/tmp/mecm2m/time.sock"
)

type Format struct {
	FormType string
}

type MecServers struct {
	Environment Environment `json:"environment"`
	MecServer   []MecServer `json:"mec-server"`
}

type PmNodes struct {
	Environment Environment `json:"environment"`
	PmNode      []PmNode    `json:"pmnode"`
}

type PsNodes struct {
	Environment Environment `json:"environment"`
	PsNode      []PsNode    `json:"psnode"`
}

type Environment struct {
	Num int `json:"num"`
}

type MecServer struct {
	Server string `json:"Server"`
	VPoint string `json:"VPoint"`
	VSNode string `json:"VSNode"`
	VMNode string `json:"VMNode"`
}

type PmNode struct {
	MServer           string `json:"MServer"`
	VPoint            string `json:"VPoint"`
	VSNode            string `json:"VSNode"`
	PSNode            string `json:"PSNode"`
	MServerConfigFile string `json:"MServer-config-file"`
}

type PsNode struct {
	PSNode     string `json:"PSNode"`
	ConfigFile string `json:"config-file"`
}

type Config struct {
	MecServers MecServers `json:"mec-servers"`
	PmNodes    PmNodes    `json:"pmnodes"`
	PsNodes    PsNodes    `json:"psnodes"`
}

func main() {
	if err := godotenv.Load(os.Getenv("HOME") + "/.env"); err != nil {
		fmt.Println("There is no ~/.env file")
	}

	// 1. 各インスタンスの登録・socketファイルの準備
	// config/register_for_neo4jの実行
	/*
		err := filepath.Walk("./config/register_for_neo4j", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() {
				// ファイルの抽出
				cmd := exec.Command("python3", path)
				output, err := cmd.CombinedOutput()
				if err != nil {
					fmt.Println(err)
				}
				fmt.Printf("%s\n", output)
			}

			return nil
		})

		if err != nil {
			panic(err)
		}
	*/

	// 2. 各プロセスファイルの実行
	// 実行系ファイルをまとめたconfigファイルを読み込む
	config_exec_file := "./config/json_files/config_main_exec_file.json"
	file, err := ioutil.ReadFile(config_exec_file)
	if err != nil {
		message.MyError(err, "Failed to read config file for exec file")
	}

	var config Config

	if err := json.Unmarshal(file, &config); err != nil {
		message.MyError(err, "Failed to unmarshal json")
	}

	// 各プロセスのプロセス番号を配列に格納しておくことで，Mainプロセスを抜けるときに，まとめておいたプロセス番号のプロセスを一斉削除できる
	processIds := []int{}

	// MEC Serverフレームワークの実行
	//mec_server_num := config.MecServers.Environment.Num

	for _, mec_server := range config.MecServers.MecServer {
		server_exec_file := mec_server.Server
		vpoint_exec_file := mec_server.VPoint
		vsnode_exec_file := mec_server.VSNode
		//vmnode_exec_file := mec_server.VMNode
		fmt.Println("----------")

		cmdServer := exec.Command(server_exec_file, os.Getenv("HOME")+os.Getenv("PROJECT_PATH")+"/MECServer/Server/socket_files/server_1.json") // 2023-05-05 ソケットファイルの指定が必須 (フルパス)
		//cmdServer := exec.Command("pwd")
		errCmdServer := cmdServer.Start()
		if errCmdServer != nil {
			message.MyError(err, "exec.Command > MEC Server > Start")
		} else {
			fmt.Println(server_exec_file, " is running")
		}

		cmdVPoint := exec.Command(vpoint_exec_file, os.Getenv("HOME")+os.Getenv("PROJECT_PATH")+"/MECServer/VPoint/socket_files/vpoint.json") // 2023-05-06 ソケットファイルの指定が必要 (フルパス)
		errCmdVPoint := cmdVPoint.Start()
		if errCmdVPoint != nil {
			message.MyError(err, "exec.Command > MEC VPoint > Start")
		} else {
			fmt.Println(vpoint_exec_file, " is running")
		}

		cmdVSNode := exec.Command(vsnode_exec_file, os.Getenv("HOME")+os.Getenv("PROJECT_PATH")+"/MECServer/VSNode/socket_files/vsnode.json") // 2023-05-06 ソケットファイルの指定が必要 (フルパス)
		errCmdVSNode := cmdVSNode.Start()
		if errCmdVSNode != nil {
			message.MyError(err, "exec.Command > MEC VSNode > Start")
		} else {
			fmt.Println(vsnode_exec_file, " is running")
		}

		/*
			cmdVMNode := exec.Command(vmnode_exec_file)
			errCmdVMNode := cmdVMNode.Start()
			if errCmdVMNode != nil {
				message.MyError(err, "exec.Command > MEC VMNode > Start")
			} else {
				fmt.Println(vmnode_exec_file, " is running")
			}
		*/

		processIds = append(processIds, cmdServer.Process.Pid, cmdVPoint.Process.Pid, cmdVSNode.Process.Pid)
	}

	// PMNodeフレームワークの実行
	//pmnode_num := config.PmNodes.Environment.Num
	/*
		for _, pmnode := range config.PmNodes.PmNode {
			mserver_exec_file := pmnode.MServer
			vpoint_exec_file := pmnode.VPoint
			vsnode_exec_file := pmnode.VSNode
			psnode_exec_file := pmnode.PSNode
			mserver_config_file := pmnode.MServerConfigFile
			fmt.Println("----------")

			cmdMServer := exec.Command(mserver_exec_file)
			errCmdMServer := cmdMServer.Start()
			if errCmdMServer != nil {
				message.MyError(err, "exec.Command > PMNode MServer > Start")
			} else {
				fmt.Println(mserver_exec_file, " is running")
			}

			cmdVPoint := exec.Command(vpoint_exec_file)
			errCmdVPoint := cmdVPoint.Start()
			if errCmdVPoint != nil {
				message.MyError(err, "exec.Command > PMNode VPoint > Start")
			} else {
				fmt.Println(vpoint_exec_file, " is running")
			}

			cmdVSNode := exec.Command(vsnode_exec_file)
			errCmdVSNode := cmdVSNode.Start()
			if errCmdVSNode != nil {
				message.MyError(err, "exec.Command > PMNode VSNode > Start")
			} else {
				fmt.Println(vsnode_exec_file, " is running")
			}

			cmdPSNode := exec.Command(psnode_exec_file)
			errCmdPSNode := cmdPSNode.Start()
			if errCmdPSNode != nil {
				message.MyError(err, "exec.Command > PMNode PSNode > Start")
			} else {
				fmt.Println(psnode_exec_file, " is running")
			}

			fmt.Println("MServer Config File: ", mserver_config_file)

			processIds = append(processIds, cmdMServer.Process.Pid, cmdVPoint.Process.Pid, cmdVSNode.Process.Pid, cmdPSNode.Process.Pid)
		}
	*/

	// PSNodeフレームワークの実行
	//psnode_num := config.PsNodes.Environment.Num

	for _, psnode := range config.PsNodes.PsNode {
		psnode_exec_file := psnode.PSNode
		config_file := psnode.ConfigFile
		fmt.Println("----------")

		cmdPSNode := exec.Command(psnode_exec_file)
		errCmdPSNode := cmdPSNode.Start()
		if errCmdPSNode != nil {
			message.MyError(err, "exec.Command > PSNode > Start")
		} else {
			fmt.Println(psnode_exec_file, " is running")
		}

		fmt.Println("PSNode Config File: ", config_file)

		processIds = append(processIds, cmdPSNode.Process.Pid)
	}
	fmt.Println("Process IDs: ", processIds)

	// main()を走らす前に，startコマンドを入力することで，各プロセスにシグナルを送信する

	// ファイルを引数にとるようなデバイス登録を実行する関数を作る．その際，ファイルを指定する
	// コマンドラインで待機しながら，プログラム開始からの時間を計測し配布することができない <- ticker() により解決
	inputChan := make(chan string)
	go ticker(inputChan)
	for {
		// クライアントがコマンドを入力
		var command string
		fmt.Printf("input command > ")
		fmt.Scan(&command)
		// APIを叩く以外のコマンドを制御
		if commandBasicExecution(command, processIds, inputChan) {
			continue
		}

		options := loadInput(command)
		sockAddr := selectSocketFile(command)
		conn, err := net.Dial(protocol, sockAddr)
		if err != nil {
			message.MyError(err, "main > net.Dial")
		}

		decoder := gob.NewDecoder(conn)
		encoder := gob.NewEncoder(conn)
		// commandExecutionする前に，Server側と型の同期を取りたい
		syncFormatClient(command, decoder, encoder)
		// APIを叩くコマンドを制御
		commandAPIExecution(command, decoder, encoder, options)
	}
}

func ticker(inputChan chan string) {
	<-inputChan
	// 時間間隔指定
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()

	// シグナル受信用チャネル
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer signal.Stop(sig)

	for {
		select {
		case now := <-t.C:
			// 現在時刻(now)の送信
			conn, err := net.Dial(protocol, timeSock)
			if err != nil {
				message.MyError(err, "ticker > net.Dial")
			}
			defer conn.Close()

			decoder := gob.NewDecoder(conn)
			encoder := gob.NewEncoder(conn)

			m := &message.MyTime{
				CurrentTime: now,
			}
			if err := encoder.Encode(m); err != nil {
				message.MyError(err, "ticker > encoder.Encode")
			}

			// 現在時刻を受信できたかどうかを受信
			if err := decoder.Decode(m); err != nil {
				message.MyError(err, "ticker > decoder.Decode")
			}
		// シグナルを受信した場合
		case s := <-sig:
			switch s {
			case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
				fmt.Println("Stop!")
				return
			}
		}
	}
}

// 入力したコマンドに対応するAPIの入力内容を取得
func loadInput(command string) []string {
	var file string
	var options []string
	switch command {
	case "point":
		// SWLat,SWLon,NELat,NELon
		file = "option_file/point.csv"
	case "node":
		// VPointID_n, Caps
		file = "option_file/node.csv"
	case "past_node":
		// VNodeID_n, Cap, Period{Start, End}
		file = "option_file/past_node.csv"
	case "past_point":
		// VPointID_n, Cap, Period{Start, End}
		file = "option_file/past_point.csv"
	case "current_node":
		// VNodeID_n, Cap
		file = "option_file/current_node.csv"
	case "current_point":
		// VPointID_n, Cap
		file = "option_file/current_point.csv"
	case "condition_node":
		// VNodeID_n, Cap, LowerLimit, UpperLimit, Timeout(s)
		file = "option_file/condition_node.csv"
	}
	fp, err := os.Open(file)
	if err != nil {
		message.MyError(err, "loadInput > os.Open")
	}
	defer fp.Close()
	r := csv.NewReader(fp)
	rows, err := r.ReadAll()
	if err != nil {
		message.MyError(err, "loadInput > r.ReadAll")
	}
	options = rows[0]
	return options
}

// 入力したコマンドに対応するソケットファイルを選択
func selectSocketFile(command string) string {
	var sockAddr string
	defaultAddr := "/tmp/mecm2m"
	defaultExt := ".sock"
	switch command {
	case "point", "node", "past_node", "past_point", "current_node", "current_point", "condition_node":
		sockAddr = defaultAddr + "/svr_1_m2mapi" + defaultExt
	default:
		sockAddr = defaultAddr + "/svr_1_m2mapi" + defaultExt
	}
	return sockAddr
}

func syncFormatClient(command string, decoder *gob.Decoder, encoder *gob.Encoder) {
	m := &Format{}
	switch command {
	case "point":
		m.FormType = "Point"
	case "node":
		m.FormType = "Node"
	case "past_node":
		m.FormType = "PastNode"
	case "past_point":
		m.FormType = "PastPoint"
	case "current_node":
		m.FormType = "CurrentNode"
	case "current_point":
		m.FormType = "CurrentPoint"
	case "condition_node":
		m.FormType = "ConditionNode"
	}
	if err := encoder.Encode(m); err != nil {
		message.MyError(err, "syncFormatClient > "+command+" > encoder.Encode")
	}
}

// APIを叩く以外のコマンドの実行 (シミュレーション実行，exit, デバイスの登録)
func commandBasicExecution(command string, processIds []int, inputChan chan string) bool {
	flag := true
	switch command {
	// シミュレーションの実行
	case "start":
		inputChan <- "start"
		for _, pid := range processIds {
			process, err := os.FindProcess(pid)
			if err != nil {
				message.MyError(err, "commandBasicExecution > start > os.FindProcess")
			}

			if err := process.Signal(syscall.Signal(syscall.SIGCONT)); err != nil {
				message.MyError(err, "commandBasicExecution > start > process.Signal")
			}
		}
		message.MyMessage("Simulating...")
		return flag
	// シミュレーションの終了
	case "exit":
		// 1. 各プロセスの削除
		for _, pid := range processIds {
			process, err := os.FindProcess(pid)
			if err != nil {
				message.MyError(err, "commandBasicExecution > exit > os.FindProcess")
			}

			err = process.Signal(os.Interrupt)
			if err != nil {
				message.MyError(err, "commandBasicExecution > exit > process.Signal")
			} else {
				fmt.Printf("process (%d) is killed\n", pid)
			}
		}

		// 2-0. パスを入手
		err := godotenv.Load(os.Getenv("HOME") + "/.env")
		if err != nil {
			message.MyError(err, "commandBasicExecution > exit > godotenv.Load")
		}

		// 2. GraphDB, SensingDBのレコード削除
		// GraphDB
		clear_graphdb_path := os.Getenv("HOME") + os.Getenv("PROJECT_PATH") + "/setup/GraphDB/clear_GraphDB.py"
		cmdGraphDB := exec.Command("python3", clear_graphdb_path)
		errCmdGraphDB := cmdGraphDB.Run()
		if errCmdGraphDB != nil {
			message.MyError(errCmdGraphDB, "commandBasicExecution > exit > cmdGraphDB.Run")
		}

		// SensingDB
		// hoge

		message.MyMessage("Bye")
		os.Exit(0)
	default:
		flag = false
		return flag
	}

	return flag
}

// APIを叩くコマンドの実行
func commandAPIExecution(command string, decoder *gob.Decoder, encoder *gob.Encoder, options []string) {
	switch command {
	case "point":
		var swlat, swlon, nelat, nelon float64
		swlat, _ = strconv.ParseFloat(options[0], 64)
		swlon, _ = strconv.ParseFloat(options[1], 64)
		nelat, _ = strconv.ParseFloat(options[2], 64)
		nelon, _ = strconv.ParseFloat(options[3], 64)
		m := &m2mapi.ResolvePoint{
			SW: m2mapi.SquarePoint{Lat: swlat, Lon: swlon},
			NE: m2mapi.SquarePoint{Lat: nelat, Lon: nelon},
		}
		if err := encoder.Encode(m); err != nil {
			message.MyError(err, "commandAPIExecution > point > encoder.Encode")
		}
		message.MyWriteMessage(m)

		// ポイント解決の結果を受信する (PsinkのVPointID_n，Address)
		ms := []m2mapi.ResolvePoint{}
		if err := decoder.Decode(&ms); err != nil {
			message.MyError(err, "commandAPIExecution > point > decoder.Decode")
		}
		message.MyReadMessage(ms)
	case "node":
		var VPointID_n string
		Caps := make([]string, len(options)-1)
		for i, option := range options {
			if i == 0 {
				VPointID_n = option
			} else {
				Caps[i-1] = option
			}
		}
		m := &m2mapi.ResolveNode{
			VPointID_n: VPointID_n,
			CapsInput:  Caps,
		}
		if err := encoder.Encode(m); err != nil {
			message.MyError(err, "commandAPIExecution > node > encoder.Encode")
		}
		message.MyWriteMessage(m)

		// ノード解決の結果を受信する（PNodeのVNodeID_n, Cap）
		ms := []m2mapi.ResolveNode{}
		if err := decoder.Decode(&ms); err != nil {
			message.MyError(err, "commandAPIExecution > node > decoder.Decode")
		}
		message.MyReadMessage(ms)
	case "past_node":
		var VNodeID_n, Capability, Start, End string
		VNodeID_n = options[0]
		Capability = options[1]
		Start = options[2]
		End = options[3]
		m := &m2mapi.ResolvePastNode{
			VNodeID_n:  VNodeID_n,
			Capability: Capability,
			Period:     m2mapi.PeriodInput{Start: Start, End: End},
		}
		if err := encoder.Encode(m); err != nil {
			message.MyError(err, "commandAPIExecution > past_node > encoder.Encode")
		}
		message.MyWriteMessage(m)

		// ノードの過去データ解決を受信する（Value, Cap, Time）
		if err := decoder.Decode(m); err != nil {
			message.MyError(err, "commandAPIExecution > past_node > decoder.Decode")
		}
		message.MyReadMessage(m)
	case "past_point":
		var VPointID_n, Capability, Start, End string
		VPointID_n = options[0]
		Capability = options[1]
		Start = options[2]
		End = options[3]
		m := &m2mapi.ResolvePastPoint{
			VPointID_n: VPointID_n,
			Capability: Capability,
			Period:     m2mapi.PeriodInput{Start: Start, End: End},
		}
		if err := encoder.Encode(m); err != nil {
			message.MyError(err, "commandAPIExecution > past_point > encoder.Encode")
		}
		message.MyWriteMessage(m)

		// ポイントの過去データ解決を受信する（VNodeID_n, Value, Cap, Time）
		if err := decoder.Decode(m); err != nil {
			message.MyError(err, "commandAPIExecution > past_point > decoder.Decode")
		}
		message.MyReadMessage(m)
	case "current_node":
		var VNodeID_n, Capability string
		VNodeID_n = options[0]
		Capability = options[1]
		m := &m2mapi.ResolveCurrentNode{
			VNodeID_n:  VNodeID_n,
			Capability: Capability,
		}
		if err := encoder.Encode(m); err != nil {
			message.MyError(err, "commandAPIExecution > current_node > encoder.Encode")
		}
		message.MyWriteMessage(m)

		// ノードの現在データ解決を受信する（Value, Cap, Time）
		if err := decoder.Decode(m); err != nil {
			message.MyError(err, "commandAPIExecution > current_node > decoder.Decode")
		}
		message.MyReadMessage(m)
	case "current_point":
		var VPointID_n, Capability string
		VPointID_n = options[0]
		Capability = options[1]
		m := &m2mapi.ResolveCurrentPoint{
			VPointID_n: VPointID_n,
			Capability: Capability,
		}
		if err := encoder.Encode(m); err != nil {
			message.MyError(err, "commandAPIExecution > current_point > encoder.Encode")
		}
		message.MyWriteMessage(m)

		// ポイントの現在データ解決を受信する（VNodeID_n, Value, Cap, Time）
		if err := decoder.Decode(m); err != nil {
			message.MyError(err, "commandAPIExecution > current_point > decoder.Decode")
		}
		message.MyReadMessage(m)
	case "condition_node":
		var VNodeID_n, Capability string
		var LowerLimit, UpperLimit float64
		var Timeout time.Duration
		VNodeID_n = options[0]
		Capability = options[1]
		LowerLimit, _ = strconv.ParseFloat(options[2], 64)
		UpperLimit, _ = strconv.ParseFloat(options[3], 64)
		Timeout, _ = time.ParseDuration(options[4])
		m := &m2mapi.ResolveConditionNode{
			VNodeID_n:  VNodeID_n,
			Capability: Capability,
			Limit:      m2mapi.Range{LowerLimit: LowerLimit, UpperLimit: UpperLimit},
			Timeout:    Timeout,
		}
		if err := encoder.Encode(m); err != nil {
			message.MyError(err, "commandAPIExecution > condition_node > encoder.Encode")
		}
		message.MyWriteMessage(m)

		// ノードの現在データを受信する（Value, Cap, Time）
		ms := m2mapi.DataForRegist{}
		if err := decoder.Decode(&ms); err != nil {
			message.MyError(err, "commandAPIExecution > condition_node > decoder.Decode")
		}
		message.MyReadMessage(ms)
	case "device_register":
		// デバイス登録を行う．
	default:

	}
}
