package main

import (
	"bufio"
	"encoding/csv"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mecm2m-Emulator/pkg/m2mapi"
	"mecm2m-Emulator/pkg/message"

	"github.com/joho/godotenv"
)

const (
	protocol           = "unix"
	timeSock           = "/tmp/mecm2m/time.sock"
	data_send_interval = 10
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
	server_num := 1
	for _, mec_server := range config.MecServers.MecServer {
		server_exec_file := mec_server.Server
		vpoint_exec_file := mec_server.VPoint
		vsnode_exec_file := mec_server.VSNode
		//vmnode_exec_file := mec_server.VMNode
		fmt.Println("----------")

		server_path := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/MECServer/Server/socket_files/server_" + strconv.Itoa(server_num) + ".json"
		cmdServer := exec.Command(server_exec_file, server_path) // 2023-05-05 ソケットファイルの指定が必須 (フルパス)
		errCmdServer := cmdServer.Start()
		if errCmdServer != nil {
			message.MyError(errCmdServer, "exec.Command > MEC Server > Start")
		} else {
			fmt.Println(server_exec_file, " is running")
		}

		vpoint_path := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/MECServer/VPoint/socket_files/vpoint_" + strconv.Itoa(server_num) + ".json"
		cmdVPoint := exec.Command(vpoint_exec_file, vpoint_path) // 2023-05-06 ソケットファイルの指定が必要 (フルパス)
		errCmdVPoint := cmdVPoint.Start()
		if errCmdVPoint != nil {
			message.MyError(errCmdVPoint, "exec.Command > MEC VPoint > Start")
		} else {
			fmt.Println(vpoint_exec_file, " is running")
		}

		vsnode_path := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/MECServer/VSNode/socket_files/vsnode_" + strconv.Itoa(server_num) + ".json"
		cmdVSNode := exec.Command(vsnode_exec_file, vsnode_path) // 2023-05-06 ソケットファイルの指定が必要 (フルパス)
		errCmdVSNode := cmdVSNode.Start()
		if errCmdVSNode != nil {
			message.MyError(errCmdVSNode, "exec.Command > MEC VSNode > Start")
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
		server_num++
	}

	// Cloud Serverの実行
	// Cloud Serverは1つとしている
	fmt.Println("----------")
	cloud_server_path := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/CloudServer/Server/socket_files/server_0.json"
	cloud_server_exec_file := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/CloudServer/Server/main"
	cmdCloudServer := exec.Command(cloud_server_exec_file, cloud_server_path) // 2023-05-05 ソケットファイルの指定が必須 (フルパス)
	errCmdCloudServer := cmdCloudServer.Start()
	if errCmdCloudServer != nil {
		message.MyError(errCmdCloudServer, "exec.Command > Cloud Server > Start")
	} else {
		fmt.Println(cloud_server_exec_file, " is running")
	}
	processIds = append(processIds, cmdCloudServer.Process.Pid)

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
	// 1 PSNode Process / MEC Server

	psnode_process_num := len(config.PsNodes.PsNode)
	psnode_num := 1
	for _, psnode := range config.PsNodes.PsNode {
		psnode_exec_file := psnode.PSNode
		config_file := psnode.ConfigFile
		fmt.Println("----------")

		psnode_path := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/PSNode/socket_files/psnode_" + strconv.Itoa(psnode_num) + ".json"
		cmdPSNode := exec.Command(psnode_exec_file, psnode_path)
		errCmdPSNode := cmdPSNode.Start()
		if errCmdPSNode != nil {
			message.MyError(errCmdPSNode, "exec.Command > PSNode > Start")
		} else {
			fmt.Println(psnode_exec_file, " is running")
		}

		fmt.Println("PSNode Config File: ", config_file)

		processIds = append(processIds, cmdPSNode.Process.Pid)
		psnode_num++
	}
	fmt.Println("Process IDs: ", processIds)

	// 3. 通信リンクプロセスの実行
	internet_link_process_exec_file := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/LinkProcess/Internet/main"
	cmdInternet := exec.Command(internet_link_process_exec_file)
	errCmdInternet := cmdInternet.Start()
	if errCmdInternet != nil {
		message.MyError(errCmdInternet, "exec.Command > Internet > Start")
	} else {
		fmt.Println(internet_link_process_exec_file, " is running")
	}
	processIds = append(processIds, cmdInternet.Process.Pid)
	fmt.Println("Internet Link Process pid: ", cmdInternet.Process.Pid)

	access_network_link_process_exec_file := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/LinkProcess/AccessNetwork/main"
	access_network_link_process_dir := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/LinkProcess/AccessNetwork"
	err = filepath.Walk(access_network_link_process_dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == ".json" {
			// ファイルの抽出
			cmdAccessNetwork := exec.Command(access_network_link_process_exec_file, path)
			errCmdAccessNetwork := cmdAccessNetwork.Start()
			if errCmdAccessNetwork != nil {
				message.MyError(errCmdAccessNetwork, "exec.Command > AccessNetwork > Start")
			} else {
				fmt.Println(path, " is running")
			}
			processIds = append(processIds, cmdAccessNetwork.Process.Pid)
			fmt.Println("Access Network Link Process pid: ", cmdAccessNetwork.Process.Pid)
		}

		return nil
	})

	if err != nil {
		panic(err)
	}

	// 4. Global/Local SensingDB のsensordataテーブルを作成する
	create_sensing_db_table_index := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/setup/SensingDB/create_db_table_index.py"
	cmdSensingDB := exec.Command("python3", create_sensing_db_table_index)
	errCmdSensingDB := cmdSensingDB.Run()
	if errCmdSensingDB != nil {
		message.MyError(errCmdSensingDB, "start > cmdSensingDB.Run")
	}

	// main()を走らす前に，startコマンドを入力することで，各プロセスにシグナルを送信する

	// ファイルを引数にとるようなデバイス登録を実行する関数を作る．その際，ファイルを指定する
	// コマンドラインで待機しながら，プログラム開始からの時間を計測し配布することができない <- ticker() により解決
	inputChan := make(chan string)
	go ticker(inputChan, psnode_process_num)

	// シミュレータ開始前
	reader := bufio.NewReader(os.Stdin)
	for {
		// クライアントがコマンドを入力
		var command string
		fmt.Printf("Inactive > ")
		command, _ = reader.ReadString('\n')

		// 何も入力がなければそのままcontinue
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}

		// シミュレーション前のコマンドを制御
		if commandExecutionBeforeEmulator(command, processIds, inputChan) {
			break
		} else {
			continue
		}
	}

	// シミュレータ開始後
	for {
		// クライアントがコマンドを入力
		var command string
		fmt.Printf("Simulating > ")
		command, _ = reader.ReadString('\n')

		// 何も入力がなければそのままcontinue
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}

		options := loadInput(command)
		sockAddr := selectSocketFile(command)
		if len(options) == 0 && sockAddr == "" {
			// コマンドが見つからない
			fmt.Println(command, ": command not found")
			continue
		} else if sockAddr == "basic command" {
			commandExecutionAfterEmulator(command, processIds)
			continue
		}
		conn, err := net.Dial(protocol, sockAddr)
		if err != nil {
			message.MyError(err, "main > net.Dial")
		}

		decoder := gob.NewDecoder(conn)
		encoder := gob.NewEncoder(conn)
		// commandExecutionする前に，Server側と型の同期を取りたい
		syncFormatClient(command, decoder, encoder)
		// APIを叩くコマンドを制御
		commandAPIExecution(command, decoder, encoder, options, processIds)
	}
}

func ticker(inputChan chan string, psnode_process_num int) {
	<-inputChan
	// 時間間隔指定
	// センサデータ登録の時間間隔を一定の閾値を設けてランダムに設定．PSNodeごとに違う時間間隔を設けたい（未完成）
	//rand.Seed(time.Now().UnixNano())
	//data_send_interval := rand.Intn(10) + 1
	t := time.NewTicker(time.Duration(data_send_interval) * time.Second)
	defer t.Stop()

	// シグナル受信用チャネル
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer signal.Stop(sig)

	for {
		select {
		case now := <-t.C:
			// 現在時刻(now)の送信
			// PSNodeプロセスの数分コネクションを張る
			go func() {
				for i := 1; i <= psnode_process_num; i++ {
					str_i := strconv.Itoa(i)
					time_socket := "/tmp/mecm2m/time_" + str_i + ".sock"
					conn, err := net.Dial(protocol, time_socket)
					if err != nil {
						message.MyError(err, "ticker > net.Dial")
					}
					defer conn.Close()

					decoder := gob.NewDecoder(conn)
					encoder := gob.NewEncoder(conn)

					myTime := &message.MyTime{
						CurrentTime: now,
					}
					if err := encoder.Encode(myTime); err != nil {
						message.MyError(err, "ticker > encoder.Encode")
					}

					// 現在時刻を受信できたかどうかを受信
					if err := decoder.Decode(myTime); err != nil {
						message.MyError(err, "ticker > decoder.Decode")
					}
				}
			}()
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
		file = "option_file/m2m_api/point.csv"
	case "node":
		// VPointID_n, Caps
		file = "option_file/m2m_api/node.csv"
	case "past_node":
		// VNodeID_n, Cap, Period{Start, End}
		file = "option_file/m2m_api/past_node.csv"
	case "past_point":
		// VPointID_n, Cap, Period{Start, End}
		file = "option_file/m2m_api/past_point.csv"
	case "current_node":
		// VNodeID_n, Cap
		file = "option_file/m2m_api/current_node.csv"
	case "current_point":
		// VPointID_n, Cap
		file = "option_file/m2m_api/current_point.csv"
	case "condition_node":
		// VNodeID_n, Cap, LowerLimit, UpperLimit, Timeout(s)
		file = "option_file/m2m_api/condition_node.csv"
	case "condition_point":
		// VPointID_n, Cap, LowerLimit, UpperLimit, Timeout(s)
		file = "option_file/m2m_api/condition_point.csv"
	case "actuate":
		// VNodeID_n, Action, Parameter
		file = "option_file/m2m_api/actuate.csv"
	case "exit", "register", "help":
		options = append(options, "basic command")
		return options
	default:
		return options
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
	case "point", "node", "past_node", "past_point", "current_node", "current_point", "condition_node", "condition_point", "actuate":
		sockAddr = defaultAddr + "/svr_1_m2mapi" + defaultExt
	case "exit", "register", "help":
		sockAddr = "basic command"
	default:
		sockAddr = ""
	}
	return sockAddr
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
	case "actuate":
		format.FormType = "Actuate"
	}
	if err := encoder.Encode(format); err != nil {
		message.MyError(err, "syncFormatClient > "+command+" > encoder.Encode")
	}
}

// APIを叩く以外のコマンドの実行 (シミュレーション実行，exit, デバイスの登録)
func commandExecutionBeforeEmulator(command string, processIds []int, inputChan chan string) bool {
	flag := false
	switch command {
	// シミュレーションの実行
	case "start":
		inputChan <- "start"
		for _, pid := range processIds {
			process, err := os.FindProcess(pid)
			if err != nil {
				message.MyError(err, "commandExecutionBeforeEmulator > start > os.FindProcess")
			}

			if err := process.Signal(syscall.Signal(syscall.SIGCONT)); err != nil {
				message.MyError(err, "commandExecutionBeforeEmulator > start > process.Signal")
			}
		}
		message.MyMessage("		*** Emulator is running *** 	")
		flag = true
		return flag
	// シミュレーションの終了
	case "exit":
		// 1. 各プロセスの削除

		for _, pid := range processIds {
			process, err := os.FindProcess(pid)
			if err != nil {
				message.MyError(err, "commandExecutionBeforeEmulator > exit > os.FindProcess")
			}

			err = process.Signal(os.Interrupt)
			if err != nil {
				message.MyError(err, "commandExecutionBeforeEmulator > exit > process.Signal")
			} else {
				fmt.Printf("process (%d) is killed\n", pid)
			}
		}

		// 2-0. パスを入手
		err := godotenv.Load(os.Getenv("HOME") + "/.env")
		if err != nil {
			message.MyError(err, "commandExecutionBeforeEmulator > exit > godotenv.Load")
		}

		// 2. GraphDB, SensingDBのレコード削除
		// GraphDB
		clear_graphdb_path := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/setup/GraphDB/clear_GraphDB.py"
		cmdGraphDB := exec.Command("python3", clear_graphdb_path)
		errCmdGraphDB := cmdGraphDB.Run()
		if errCmdGraphDB != nil {
			message.MyError(errCmdGraphDB, "commandExecutionBeforeEmulator > exit > cmdGraphDB.Run")
		}

		// SensingDB
		clear_sensingdb_path := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/setup/SensingDB/clear_SensingDB.py"
		cmdSensingDB := exec.Command("python3", clear_sensingdb_path)
		errCmdSensingDB := cmdSensingDB.Run()
		if errCmdSensingDB != nil {
			message.MyError(errCmdSensingDB, "commandExecutionBeforeEmulator > exit > cmdSensingDB.Run")
		}

		message.MyMessage("Bye")
		os.Exit(0)
	// デバイスの登録
	case "register":
		// あらかじめ，登録するデバイスの情報をconfigファイルとして用意しておく
		// ./option_file/register/register_psnode.py を実行
		// 2023-05-10 一旦PSNodeの登録のみ
		register_psnode_script := "./option_file/register/register_psnode.py"
		register_psnode_file := "./option_file/register/register_psnode.json"
		cmdGraphDB := exec.Command("python3", register_psnode_script, register_psnode_file)
		errCmdGraphDB := cmdGraphDB.Run()
		if errCmdGraphDB != nil {
			message.MyError(errCmdGraphDB, "commandExecutionBeforeEmulator > register > cmdGraphDB.Run")
		}

		message.MyMessage("		*** Succeed: Register device *** 	")
		return flag
	// helpコマンド
	case "help":
		fmt.Println("[start]: 	Start MECM2M Emulator")
		fmt.Println("[exit]: 	Exit MECM2M Emulator")
		fmt.Println("[register]: 	Register PSNode")
		return flag
	default:
		// コマンドが見つからない
		fmt.Println(command, ": command not found")
		return flag
	}

	return flag
}

// APIを叩く以外のコマンドの実行 (シミュレーション実行，exit, デバイスの登録)
func commandExecutionAfterEmulator(command string, processIds []int) {
	switch command {
	// シミュレーションの終了
	case "exit":
		// 1. 各プロセスの削除
		for _, pid := range processIds {
			process, err := os.FindProcess(pid)
			if err != nil {
				message.MyError(err, "commandExecutionAfterEmulator > exit > os.FindProcess")
			}

			err = process.Signal(os.Interrupt)
			if err != nil {
				message.MyError(err, "commandExecutionAfterEmulator > exit > process.Signal")
			} else {
				fmt.Printf("process (%d) is killed\n", pid)
			}
		}
		// 2-0. パスを入手
		err := godotenv.Load(os.Getenv("HOME") + "/.env")
		if err != nil {
			message.MyError(err, "commandExecutionAfterEmulator > exit > godotenv.Load")
		}

		// 2. GraphDB, SensingDBのレコード削除
		// GraphDB
		clear_graphdb_path := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/setup/GraphDB/clear_GraphDB.py"
		cmdGraphDB := exec.Command("python3", clear_graphdb_path)
		errCmdGraphDB := cmdGraphDB.Run()
		if errCmdGraphDB != nil {
			message.MyError(errCmdGraphDB, "commandExecutionAfterEmulator > exit > cmdGraphDB.Run")
		}

		// SensingDB
		clear_sensingdb_path := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/setup/SensingDB/clear_SensingDB.py"
		cmdSensingDB := exec.Command("python3", clear_sensingdb_path)
		errCmdSensingDB := cmdSensingDB.Run()
		if errCmdSensingDB != nil {
			message.MyError(errCmdSensingDB, "commandExecutionAfterEmulator > exit > cmdSensingDB.Run")
		}
		message.MyMessage("Bye")
		os.Exit(0)
	// デバイスの登録
	case "register":
		// あらかじめ，登録するデバイスの情報をconfigファイルとして用意しておく
		// ./option_file/register/register_psnode.py を実行
		// 2023-05-10 一旦PSNodeの登録のみ
		// 本来はこの場所ではない．APIExecutionに入れる (2023-05-25)
		register_psnode_script := "./option_file/register/register_psnode.py"
		register_psnode_file := "./option_file/register/register_psnode.json"
		cmdGraphDB := exec.Command("python3", register_psnode_script, register_psnode_file)
		errCmdGraphDB := cmdGraphDB.Run()
		if errCmdGraphDB != nil {
			message.MyError(errCmdGraphDB, "commandExecutionAfterEmulator > register > cmdGraphDB.Run")
		}

		message.MyMessage("		*** Succeed: Register device *** 	")
	// helpコマンド
	case "help":
		fmt.Println("[exit]: 		Exit MECM2M Emulator")
		fmt.Println("[register]: 		Register PSNode")
		fmt.Println("[point]: 		Resolve Point")
		fmt.Println("[node]: 		Resolve Node")
		fmt.Println("[past_node]: 		Resolve Past Data By Node")
		fmt.Println("[current_node]: 	Resolve Current Data By Node")
		fmt.Println("[past_point]: 		Resolbe Past Data By Point")
		fmt.Println("[current_point]: 	Resolve Current Data By Point")
		fmt.Println("[actuate]:		Actuate a Node")
	default:
		// コマンドが見つからない
		fmt.Println(command, ": command not found")
	}
}

// APIを叩くコマンドの実行
func commandAPIExecution(command string, decoder *gob.Decoder, encoder *gob.Encoder, options []string, processIds []int) {
	switch command {
	case "point":
		var swlat, swlon, nelat, nelon float64
		swlat, _ = strconv.ParseFloat(options[0], 64)
		swlon, _ = strconv.ParseFloat(options[1], 64)
		nelat, _ = strconv.ParseFloat(options[2], 64)
		nelon, _ = strconv.ParseFloat(options[3], 64)
		point_input := &m2mapi.ResolvePoint{
			SW: m2mapi.SquarePoint{Lat: swlat, Lon: swlon},
			NE: m2mapi.SquarePoint{Lat: nelat, Lon: nelon},
		}
		if err := encoder.Encode(point_input); err != nil {
			message.MyError(err, "commandAPIExecution > point > encoder.Encode")
		}
		message.MyWriteMessage(*point_input)

		// ポイント解決の結果を受信する (PsinkのVPointID_n，Address)
		point_output := []m2mapi.ResolvePoint{}
		if err := decoder.Decode(&point_output); err != nil {
			message.MyError(err, "commandAPIExecution > point > decoder.Decode")
		}
		message.MyReadMessage(point_output)
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
		node_input := &m2mapi.ResolveNode{
			VPointID_n: VPointID_n,
			CapsInput:  Caps,
		}
		if err := encoder.Encode(node_input); err != nil {
			message.MyError(err, "commandAPIExecution > node > encoder.Encode")
		}
		message.MyWriteMessage(*node_input)

		// ノード解決の結果を受信する（PNodeのVNodeID_n, Cap）
		node_output := []m2mapi.ResolveNode{}
		if err := decoder.Decode(&node_output); err != nil {
			message.MyError(err, "commandAPIExecution > node > decoder.Decode")
		}
		message.MyReadMessage(node_output)
	case "past_node":
		var VNodeID_n, Capability, Start, End, SocketAddress string
		VNodeID_n = options[0]
		Capability = options[1]
		Start = options[2]
		End = options[3]
		SocketAddress = options[4]
		past_node_input := &m2mapi.ResolvePastNode{
			VNodeID_n:     VNodeID_n,
			Capability:    Capability,
			Period:        m2mapi.PeriodInput{Start: Start, End: End},
			SocketAddress: SocketAddress,
		}
		if err := encoder.Encode(past_node_input); err != nil {
			message.MyError(err, "commandAPIExecution > past_node > encoder.Encode")
		}
		message.MyWriteMessage(*past_node_input)

		// ノードの過去データ解決を受信する（VNodeID_n, Value, Cap, Time）
		past_node_output := m2mapi.ResolvePastNode{}
		if err := decoder.Decode(&past_node_output); err != nil {
			message.MyError(err, "commandAPIExecution > past_node > decoder.Decode")
		}
		message.MyReadMessage(past_node_output)
	case "past_point":
		var VPointID_n, Capability, Start, End, SocketAddress string
		VPointID_n = options[0]
		Capability = options[1]
		Start = options[2]
		End = options[3]
		SocketAddress = options[4]
		past_point_input := &m2mapi.ResolvePastPoint{
			VPointID_n:    VPointID_n,
			Capability:    Capability,
			Period:        m2mapi.PeriodInput{Start: Start, End: End},
			SocketAddress: SocketAddress,
		}
		if err := encoder.Encode(past_point_input); err != nil {
			message.MyError(err, "commandAPIExecution > past_point > encoder.Encode")
		}
		message.MyWriteMessage(*past_point_input)

		// ポイントの過去データ解決を受信する（VNodeID_n, Value, Cap, Time）
		past_point_output := m2mapi.ResolvePastPoint{}
		if err := decoder.Decode(&past_point_output); err != nil {
			message.MyError(err, "commandAPIExecution > past_point > decoder.Decode")
		}
		message.MyReadMessage(past_point_output)
	case "current_node":
		var VNodeID_n, Capability, SocketAddress string
		VNodeID_n = options[0]
		Capability = options[1]
		SocketAddress = options[2]
		current_node_input := &m2mapi.ResolveCurrentNode{
			VNodeID_n:     VNodeID_n,
			Capability:    Capability,
			SocketAddress: SocketAddress,
		}
		if err := encoder.Encode(current_node_input); err != nil {
			message.MyError(err, "commandAPIExecution > current_node > encoder.Encode")
		}
		message.MyWriteMessage(current_node_input)

		// ノードの現在データ解決を受信する（Value, Cap, Time)
		current_node_output := m2mapi.ResolveCurrentNode{}
		if err := decoder.Decode(&current_node_output); err != nil {
			message.MyError(err, "commandAPIExecution > current_node > decoder.Decode")
		}
		message.MyReadMessage(current_node_output)
	case "current_point":
		var VPointID_n, Capability, SocketAddress string
		VPointID_n = options[0]
		Capability = options[1]
		SocketAddress = options[2]
		current_point_input := &m2mapi.ResolveCurrentPoint{
			VPointID_n:    VPointID_n,
			Capability:    Capability,
			SocketAddress: SocketAddress,
		}
		if err := encoder.Encode(current_point_input); err != nil {
			message.MyError(err, "commandAPIExecution > current_point > encoder.Encode")
		}
		message.MyWriteMessage(current_point_input)

		// ポイントの現在データ解決を受信する（VNodeID_n, Value, Cap, Time）
		current_point_output := m2mapi.ResolveCurrentPoint{}
		if err := decoder.Decode(&current_point_output); err != nil {
			message.MyError(err, "commandAPIExecution > current_point > decoder.Decode")
		}
		message.MyReadMessage(current_point_output)
	case "condition_node":
		var VNodeID_n, Capability, SocketAddress string
		var LowerLimit, UpperLimit float64
		var Timeout time.Duration
		VNodeID_n = options[0]
		Capability = options[1]
		LowerLimit, _ = strconv.ParseFloat(options[2], 64)
		UpperLimit, _ = strconv.ParseFloat(options[3], 64)
		Timeout, _ = time.ParseDuration(options[4])
		SocketAddress = options[5]
		condition_node_input := &m2mapi.ResolveConditionNode{
			VNodeID_n:     VNodeID_n,
			Capability:    Capability,
			Limit:         m2mapi.Range{LowerLimit: LowerLimit, UpperLimit: UpperLimit},
			Timeout:       Timeout,
			SocketAddress: SocketAddress,
		}
		if err := encoder.Encode(condition_node_input); err != nil {
			message.MyError(err, "commandAPIExecution > condition_node > encoder.Encode")
		}
		message.MyWriteMessage(condition_node_input)

		// ノードの現在データを受信する（Value, Cap, Time）
		condition_node_output := m2mapi.DataForRegist{}
		if err := decoder.Decode(&condition_node_output); err != nil {
			message.MyError(err, "commandAPIExecution > condition_node > decoder.Decode")
		}
		message.MyReadMessage(condition_node_output)
	case "condition_point":
		var VPointID_n, Capability, SocketAddress string
		var LowerLimit, UpperLimit float64
		var Timeout time.Duration
		VPointID_n = options[0]
		Capability = options[1]
		LowerLimit, _ = strconv.ParseFloat(options[2], 64)
		UpperLimit, _ = strconv.ParseFloat(options[3], 64)
		Timeout, _ = time.ParseDuration(options[4])
		SocketAddress = options[5]
		condition_point_input := &m2mapi.ResolveConditionPoint{
			VPointID_n:    VPointID_n,
			Capability:    Capability,
			Limit:         m2mapi.Range{LowerLimit: LowerLimit, UpperLimit: UpperLimit},
			Timeout:       Timeout,
			SocketAddress: SocketAddress,
		}
		if err := encoder.Encode(condition_point_input); err != nil {
			message.MyError(err, "commandAPIExecution > condition_point > encoder.Encode")
		}
		message.MyWriteMessage(condition_point_input)

		// ノードの現在データを受信する（Value, Cap, Time）
		condition_point_output := m2mapi.DataForRegist{}
		if err := decoder.Decode(&condition_point_output); err != nil {
			message.MyError(err, "commandAPIExecution > condition_point > decoder.Decode")
		}
		message.MyReadMessage(condition_point_output)
	case "actuate":
		var VNodeID_n, Action, SocketAddress string
		var Parameter float64
		VNodeID_n = options[0]
		Action = options[1]
		Parameter, _ = strconv.ParseFloat(options[2], 64)
		SocketAddress = options[3]
		actuate_input := &m2mapi.Actuate{
			VNodeID_n:     VNodeID_n,
			Action:        Action,
			Parameter:     Parameter,
			SocketAddress: SocketAddress,
		}
		if err := encoder.Encode(actuate_input); err != nil {
			message.MyError(err, "commandAPIExecution > actuate > encoder.Encode")
		}
		message.MyWriteMessage(actuate_input)

		// アクチュエートによる状態を受信する (Status)
		actuate_output := m2mapi.Actuate{}
		if err := decoder.Decode(&actuate_output); err != nil {
			message.MyError(err, "commandAPIExecution > actuate > decoder.Decode")
		}
		message.MyReadMessage(actuate_output)
	default:
		// コマンドが見つからない
		fmt.Println(command, ": command not found")
	}
}
