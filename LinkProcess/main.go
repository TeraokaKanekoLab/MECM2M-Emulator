package main

import (
	"bytes"
	"encoding/csv"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mecm2m-Emulator/pkg/message"
	"mecm2m-Emulator/pkg/mserver"
	"mecm2m-Emulator/pkg/server"
	"mecm2m-Emulator/pkg/vpoint"
	"mecm2m-Emulator/pkg/vsnode"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

const (
	protocol                 = "unix"
	link_socket_address_root = "/tmp/mecm2m/link-process/"
	concurrency              = 3600
)

var (
	rtt_file string
)

type Format struct {
	FormType string
}

type LinkProcess struct {
	SocketAddresses []string `json:"socket_addresses"`
}

func cleanup(socketFiles ...string) {
	for _, sock := range socketFiles {
		if _, err := os.Stat(sock); err == nil {
			if err := os.RemoveAll(sock); err != nil {
				message.MyError(err, "cleanup > os.RemoveAll")
			}
		}
	}
}

func init() {
	// .envファイルの読み込み
	if err := godotenv.Load(os.Getenv("HOME") + "/.env"); err != nil {
		log.Fatal(err)
	}
	rtt_file = os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/LinkProcess/rtt.csv"
}

func main() {
	/*
		// Mainプロセスのコマンドラインからシミュレーション実行開始シグナルを受信するまで待機
		signals_from_main := make(chan os.Signal, 1)

		// 停止しているプロセスを再開するために送信されるシグナル，SIGCONT(=18)を受信するように設定
		signal.Notify(signals_from_main, syscall.SIGCONT)

		// シグナルを待機
		fmt.Println("Waiting for signal...")
		sig := <-signals_from_main

		// 受信したシグナルを表示
		fmt.Printf("Received signal: %v\n", sig)
	*/
	var socketFiles []string

	config_link_process := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/LinkProcess/access_network_link_process.json"
	file, err := os.ReadFile(config_link_process)
	if err != nil {
		message.MyError(err, "Failed to read config file for link process")
	}

	var linkProcess LinkProcess

	if err := json.Unmarshal(file, &linkProcess); err != nil {
		message.MyError(err, "Failed to unmarshal json")
	}
	socketFiles = append(socketFiles, linkProcess.SocketAddresses...)
	cleanup(socketFiles...)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGALRM, syscall.SIGTERM)
	go func() {
		<-quit
		fmt.Println("ctrl-c pressed!")
		close(quit)
		cleanup(socketFiles...)
		os.Exit(0)
	}()

	var wg sync.WaitGroup

	sem := make(chan struct{}, concurrency)

	fmt.Println("Starting Server")

	for _, file := range socketFiles {
		sem <- struct{}{}

		wg.Add(1)
		go initialize(file, &wg, sem)
	}

	defer close(sem)
	wg.Wait()
}

func initialize(file string, wg *sync.WaitGroup, sem chan struct{}) {
	defer wg.Done()
	defer func() { <-sem }()
	listener, err := net.Listen(protocol, file)
	if err != nil {
		message.MyError(err, "initialize > net.Listen")
	}
	defer listener.Close()

	controlChannel := make(chan os.Signal, 1)
	signal.Notify(controlChannel, syscall.SIGCONT)

	var wg1 sync.WaitGroup
	wg1.Add(2)

	go func() {
		defer wg1.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				message.MyError(err, "initialize > listener.Accept")
				break
			}

			go connectionLink(conn, file)
		}
	}()

	go func() {
		defer wg1.Done()
		for {
			select {
			case <-controlChannel:
				listener.Close()
				listener, err = net.Listen(protocol, file)
				if err != nil {
					message.MyError(err, "initialize > net.Listen")
				}
				defer listener.Close()

			}
		}
	}()
	wg.Wait()
}

func connectionLink(conn net.Conn, file string) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	// message.MyMessage("[MESSAGE] Call Link Process thread")

	for {
		// 型同期をして，型の種類に応じてスイッチ
		switch psnodeCommand := syncFormatServer(decoder, encoder); psnodeCommand.(type) {
		case *vsnode.ResolveCurrentDataByNode:
			format := psnodeCommand.(*vsnode.ResolveCurrentDataByNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					// message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "connectionLink > CurrentNode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// RTT時間を検索して，RTT/2 時間を取得
			rtt_half := searchRTT(format.PNodeID)
			// fmt.Println("RTT: ", rtt_half)

			// RTT/2 時間待機
			delayRTTHalf(rtt_half)

			// 宛先ソケットアドレス用の通信経路を確立 (クライアント側)．PSNodeはIP:Port
			// 入力のVNodeIDから宛先のPSNodeのPort番号を割り出す
			psnode_port := trimPSNodePort(format.PNodeID)
			transmit_data, err := json.Marshal(format)
			if err != nil {
				fmt.Println("Error marshalling data: ", err)
				return
			}
			response, err := http.Post("http://localhost:"+psnode_port+"/devapi/data/current/node", "application/json", bytes.NewBuffer(transmit_data))
			if err != nil {
				fmt.Println("Error making request:", err)
				return
			}
			defer response.Body.Close()

			body, err := io.ReadAll(response.Body)
			if err != nil {
				panic(err)
			}
			var current_node_output vsnode.ResolveCurrentDataByNode
			if err = json.Unmarshal(body, &current_node_output); err != nil {
				fmt.Println("Error Unmarshaling: ", err)
				return
			}

			// RTT/2 時間待機
			delayRTTHalf(rtt_half)

			// 送信元に結果を転送
			if err := encoder.Encode(&current_node_output); err != nil {
				message.MyError(err, "connectionLink > CurrentNode > encoder.Encode")
			}
			message.MyWriteMessage(current_node_output)
		case *vsnode.Actuate:
			format := psnodeCommand.(*vsnode.Actuate)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					// message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "connectionLink > Actuate > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// RTT時間を検索して，RTT/2 時間を取得
			rtt_half := searchRTT(format.PNodeID)
			// fmt.Println("RTT: ", rtt_half)

			// RTT/2 時間待機
			delayRTTHalf(rtt_half)

			// 宛先ソケットアドレス用の通信経路を確立
			// 入力のVNodeIDから宛先のPSNodeのPort番号を割り出す
			psnode_port := trimPSNodePort(format.PNodeID)
			transmit_data, err := json.Marshal(format)
			if err != nil {
				fmt.Println("Error marshalling data: ", err)
				return
			}
			response, err := http.Post("http://localhost:"+psnode_port+"/devapi/actuate", "application/json", bytes.NewBuffer(transmit_data))
			if err != nil {
				fmt.Println("Error making request:", err)
				return
			}
			defer response.Body.Close()

			body, err := io.ReadAll(response.Body)
			if err != nil {
				panic(err)
			}
			var actuate_output vsnode.Actuate
			if err = json.Unmarshal(body, &actuate_output); err != nil {
				fmt.Println("Error Unmarshaling: ", err)
				return
			}

			// RTT/2 時間待機
			delayRTTHalf(rtt_half)

			// 送信元に結果を転送
			if err := encoder.Encode(&actuate_output); err != nil {
				message.MyError(err, "connectionLink > Actuate > encoder.Encode")
			}
			message.MyWriteMessage(actuate_output)
		}
	}
}

// 開いているソケットファイル名からsrc, dstのサーバを割り出す
func searchPNodeID(file string) string {
	start_index := strings.LastIndex(file, "_")
	last_index := strings.LastIndex(file, ".")
	pnode_id := file[start_index+1 : last_index]
	return pnode_id
}

// 通信間のサーバとRTTの組をまとめたファイルからRTT時間の検索
func searchRTT(pnode_id string) time.Duration {
	var rtt_half time.Duration
	rtt_fp, err := os.Open(rtt_file)
	if err != nil {
		message.MyError(err, "RTT file cannot open")
	}
	defer rtt_fp.Close()

	reader := csv.NewReader(rtt_fp)
	records, err := reader.ReadAll()
	if err != nil {
		message.MyError(err, "RTT file cannot read")
	}

	for _, record := range records {
		if record[0] == pnode_id {
			rtt_float, _ := strconv.ParseFloat(record[1], 64)
			rtt_half_float := rtt_float / 2
			rtt_half_str := strconv.FormatFloat(rtt_half_float, 'f', 2, 64) + "ms"
			rtt_half, _ = time.ParseDuration(rtt_half_str)
		}
	}
	return rtt_half
}

// RTT/2 時間待機する
func delayRTTHalf(rtt_half time.Duration) {
	time.Sleep(rtt_half)
}

// 型同期をするための関数
func syncFormatServer(decoder *gob.Decoder, encoder *gob.Encoder) any {
	format := &Format{}
	if err := decoder.Decode(format); err != nil {
		if err == io.EOF {
			typeM := "exit"
			return typeM
		} else {
			message.MyError(err, "syncFormatServer > decoder.Decode")
		}
	}
	typeResult := format.FormType

	var typeM any
	switch typeResult {
	case "CurrentNode":
		typeM = &vsnode.ResolveCurrentDataByNode{}
	case "Actuate":
		typeM = &vsnode.Actuate{}
	case "ConnectNew":
		typeM = &mserver.ConnectNew{}
	case "ConnectForModule":
		typeM = &mserver.ConnectForModule{}
	case "AAA":
		typeM = &server.AAA{}
	case "Disconn":
		typeM = &mserver.Disconnect{}
	case "SessionKey":
		typeM = &server.RequestSessionKey{}
	case "CurrentPointVNode":
		typeM = &vpoint.CurrentPointVNode{}
	default:
	}
	return typeM
}

func trimPSNodePort(vnodeid string) string {
	vnodeid_int, _ := strconv.ParseUint(vnodeid, 10, 64)
	base_port_int, _ := strconv.Atoi(os.Getenv("PSNODE_BASE_PORT"))
	mask := uint64(1<<60 - 1)
	id_index := vnodeid_int & mask
	port := strconv.Itoa(base_port_int + int(id_index))
	return port
}
