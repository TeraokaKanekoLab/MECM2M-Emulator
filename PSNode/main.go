package main

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"math/rand"
	"mecm2m-Emulator/pkg/m2mapi"
	"mecm2m-Emulator/pkg/message"
	"mecm2m-Emulator/pkg/psnode"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

const (
	protocol            = "unix"
	layout              = "2006-01-02 15:04:05"
	timeSock            = "/tmp/mecm2m/time.sock"
	dataResisterSock    = "/tmp/mecm2m/data_resister.sock"
	socket_address_root = "/tmp/mecm2m/"
)

type Format struct {
	FormType string
}

type CurrentTime struct {
}

var currentTime CurrentTime

func cleanup(socketFiles ...string) {
	for _, sock := range socketFiles {
		if _, err := os.Stat(sock); err == nil {
			if err := os.RemoveAll(sock); err != nil {
				message.MyError(err, "cleanup > os.RemoveAll")
			}
		}
	}
}

func main() {
	loadEnv()
	//configファイルを読み込んで，センサデータ送信用のデータ，センサデータ登録用のデータを読み込む
	// Mainプロセスのコマンドラインからシミュレーション実行開始シグナルを受信するまで待機

	// コマンドライン引数にソケットファイル群をまとめたファイルをしていして，初めにそのファイルを読み込む
	if len(os.Args) != 2 {
		fmt.Println("There is no socket files")
		os.Exit(1)
	}

	// Mainプロセスのコマンドラインからシミュレーション実行開始シグナルを受信するまで待機
	signals_from_main := make(chan os.Signal, 1)

	// 停止しているプロセスを再開するために送信されるシグナル，SIGCONT(=18)を受信するように設定
	signal.Notify(signals_from_main, syscall.SIGCONT)

	// シグナルを待機
	fmt.Println("Waiting for signal...")
	sig := <-signals_from_main

	// 受信したシグナルを表示
	fmt.Printf("Received signal: %v\n", sig)

	socket_file_name := os.Args[1]
	data, err := ioutil.ReadFile(socket_file_name)
	if err != nil {
		message.MyError(err, "Failed to read socket file")
	}

	var socket_files psnode.PSNodeSocketFiles

	if err := json.Unmarshal(data, &socket_files); err != nil {
		message.MyError(err, "Failed to unmarshal json")
	}

	// PSNodeをいくつか用意しておく
	var socketFiles []string
	socketFiles = append(socketFiles, socket_files.PSNodes...)
	gids := make(chan uint64, len(socketFiles))
	cleanup(socketFiles...)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	go func() {
		<-quit
		fmt.Println("ctrl-c pressed!")
		close(quit)
		cleanup(socketFiles...)
		cleanup(timeSock)
		os.Exit(0)
	}()

	//ここでmainプロセスから時刻を受信する
	mainContext := context.Background()
	retTime := make(chan time.Time, 1)
	go timeSync(mainContext, retTime)
	go func(retTime chan time.Time) {
		for {
			select {
			case t := <-retTime:
				fmt.Println("It's now... ", t)
				//PSNodeごとに周期を変えられるようにしたい
				for _, file := range socketFiles {
					go registerSensingData(file, t)
				}
			default:
				time.Sleep(1 * time.Millisecond)
			}
		}
	}(retTime)

	//psnodeの実行

	var wg sync.WaitGroup

	for _, file := range socketFiles {
		wg.Add(1)
		go func(file string, wg *sync.WaitGroup) {
			defer wg.Done()
			gids <- getGID()
			gid := getGID()
			fmt.Printf("GOROUTINE ID (%s): %d\n", file, gid)
			listener, err := net.Listen(protocol, file)
			if err != nil {
				message.MyError(err, "main > net.Listen")
			}
			message.MyMessage("> [Initialize] Socket file launched: " + file)

			for {
				conn, err := listener.Accept()
				if err != nil {
					message.MyError(err, "main > listener.Accept")
					break
				}
				go psnodes(conn, gid)
			}
		}(file, &wg)
	}
	wg.Wait()
	cleanup(timeSock)
	defer close(gids)
}

// mainプロセスからの時刻配布を受信・所定の一定時間間隔でSensingDBにセンサデータ登録
func timeSync(mainContext context.Context, retTime chan time.Time) {
	listener, err := net.Listen(protocol, timeSock)
	if err != nil {
		message.MyError(err, "timeSync > net.Listen")
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			message.MyError(err, "timeSync > listener.Accept")
			break
		}
		defer conn.Close()

		decoderTime := gob.NewDecoder(conn)
		encoderTime := gob.NewEncoder(conn)
		message.MyMessage("[MESSAGE] Call timeSync thread")

		go func(retTime chan time.Time) {
			for {
				//時刻型を持つ変数の定義
				myTime := &message.MyTime{}
				if err := decoderTime.Decode(myTime); err != nil {
					if err == io.EOF {
						break
					}
					message.MyError(err, "timeSync > decoderTime.Decode")
					break
				}

				getTimeContext := context.WithValue(mainContext, currentTime, myTime.CurrentTime)
				var t time.Time
				switch getTimeContext.Value(currentTime).(type) {
				case time.Time:
					myTime.Ack = true
					t = getTimeContext.Value(currentTime).(time.Time)
				default:
					myTime.Ack = false
				}
				if err := encoderTime.Encode(myTime); err != nil {
					message.MyError(err, "timeSync > encoderTime.Encode")
					break
				}
				retTime <- t
			}
		}(retTime)
	}
}

// センサデータ送信，センサデータ登録
func psnodes(conn net.Conn, gid uint64) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	message.MyMessage("[MESSAGE] Call PSNode thread(" + strconv.FormatUint(gid, 10) + ")")

	for {
		//VSNodeとやりとりをする初めに型の同期をとる
		switch psnodesCommand := syncFormatServer(decoder, encoder); psnodesCommand.(type) {
		case *m2mapi.ResolveCurrentNode:
			format := psnodesCommand.(*m2mapi.ResolveCurrentNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "psnode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			currentTime := time.Now()
			//configファイルからセンサデータ読み込む
			current_node_output := m2mapi.ResolveCurrentNode{}
			current_node_output.VNodeID_n = format.VNodeID_n
			rand.Seed(time.Now().UnixNano())
			value_min := 30.0
			value_max := 40.0
			value_float := value_min + rand.Float64()*(value_max-value_min)
			current_node_output.Values = m2mapi.Value{Capability: format.Capability, Time: currentTime.Format(layout), Value: value_float}
			//センサデータをVSNodeに送信する
			if err := encoder.Encode(&current_node_output); err != nil {
				message.MyError(err, "vsnode > CurrentNode > encoder.Encode")
				break
			}
			message.MyWriteMessage(current_node_output)
		case *m2mapi.Actuate:
			format := psnodesCommand.(*m2mapi.Actuate)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "psnode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// アクチュエータが正常に動作したことを返す
			actuate_output := m2mapi.Actuate{}
			actuate_output.Status = true

			// 状態結果をVSNodeに送信する
			if err := encoder.Encode(&actuate_output); err != nil {
				message.MyError(err, "vsnode > Actuate > encoder.Encode")
				break
			}
			message.MyWriteMessage(actuate_output)
		}
	}

}

// センサデータの登録
// PSNode -> VSNode -> SensingDB
func registerSensingData(file string, t time.Time) {
	// センサデータ登録に必要な情報の定義
	var pnode_id, capability, psink_id string
	var value, lat, lon float64
	var t_now time.Time
	var psnode_psink_label string

	// PSNodeのconfigファイルを検索し，ソケットファイルと一致する情報を取得する
	psnode_json_file_path := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/Main/config/json_files/config_main_psnode.json"
	psnodeJsonFile, err := os.Open(psnode_json_file_path)
	if err != nil {
		fmt.Println(err)
	}
	defer psnodeJsonFile.Close()
	psnodeByteValue, _ := ioutil.ReadAll(psnodeJsonFile)

	var psnodeResult map[string][]interface{}
	json.Unmarshal(psnodeByteValue, &psnodeResult)

	// psnodeソケットファイルから，vsnodeソケットファイルを生成
	psnode_socket_file_first_underscore_index := strings.Index(file, "_")
	psnode_socket_file_last_underscore_index := strings.LastIndex(file, "_")
	psnode_socket_file_extension_index := strings.Index(file, ".")
	psnode_server_num_from_socket_file := file[psnode_socket_file_first_underscore_index+1 : psnode_socket_file_last_underscore_index]
	psnode_id_num_from_socket_file := file[psnode_socket_file_last_underscore_index+1 : psnode_socket_file_extension_index]

	vsnode_id_from_socket_file := convertID(psnode_id_num_from_socket_file, 63)
	vsnode_socket_address_from_psnode := socket_address_root + "vsnode_" + psnode_server_num_from_socket_file + "_" + vsnode_id_from_socket_file + ".sock"

	psnodes := psnodeResult["psnodes"]
	for _, v := range psnodes {
		psnode_format := v.(map[string]interface{})
		psnode := psnode_format["psnode"].(map[string]interface{})
		psnode_data_property := psnode["data-property"].(map[string]interface{})
		psnode_relation_label := psnode["relation-label"].(map[string]interface{})
		vsnode_socket_address := psnode_data_property["SocketAddress"].(string)
		if vsnode_socket_address == vsnode_socket_address_from_psnode {
			pnode_id = psnode_data_property["PNodeID"].(string)
			capability = psnode_data_property["Capability"].(string)
			psnode_psink_label = psnode_relation_label["PSink"].(string)
			position_interface := psnode_data_property["Position"].([]interface{})
			lat = position_interface[0].(float64)
			lon = position_interface[1].(float64)
			break
		}
	}

	// PSinkのconfigファイルを検索し，ソケットファイルと一致する情報を取得する
	psink_json_file_path := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/Main/config/json_files/config_main_psink.json"
	psinkJsonFile, err := os.Open(psink_json_file_path)
	if err != nil {
		fmt.Println(err)
	}
	defer psinkJsonFile.Close()
	psinkByteValue, _ := ioutil.ReadAll(psinkJsonFile)

	var psinkResult map[string][]interface{}
	json.Unmarshal(psinkByteValue, &psinkResult)

	psinks := psinkResult["psinks"]
	for _, v := range psinks {
		psink_format := v.(map[string]interface{})
		psink := psink_format["psink"].(map[string]interface{})
		psink_data_property := psink["data-property"].(map[string]interface{})
		psink_label := psink_data_property["Label"].(string)
		if psink_label == psnode_psink_label {
			psink_id = psink_data_property["PSinkID"].(string)
			break
		}
	}

	t_now = time.Now()
	rand.Seed(time.Now().UnixNano())
	value_min := 30.0
	value_max := 40.0
	value = value_min + rand.Float64()*(value_max-value_min)

	// センサデータ登録用の型を指定
	m := &m2mapi.DataForRegist{
		PNodeID:    pnode_id,
		Capability: capability,
		Timestamp:  t_now.Format(layout),
		Value:      value,
		PSinkID:    psink_id,
		Lat:        lat,
		Lon:        lon,
	}

	// VSNodeへ接続
	// ソケットファイルは自身のソケットファイルのID部分をビット変換
	connDB, err := net.Dial(protocol, dataResisterSock)
	if err != nil {
		message.MyError(err, "registerSensingDB > net.Dial")
	}
	decoderDB := gob.NewDecoder(connDB)
	encoderDB := gob.NewEncoder(connDB)

	syncFormatClient("RegisterSensingData", decoderDB, encoderDB)

	if err := encoderDB.Encode(m); err != nil {
		message.MyError(err, "registerSensingData > encoderDB.Encode")
	}
	fmt.Println("Data Register Request at...", t)
	fmt.Println("Data Register Response at...", t_now)
}

// VSNodeと型同期をするための関数
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
	case "CurrentNode", "CurrentPoint":
		typeM = &m2mapi.ResolveCurrentNode{}
	case "Actuate":
		typeM = &m2mapi.Actuate{}
	}
	return typeM
}

// SensingDBと型同期をするための関数
func syncFormatClient(command string, decoder *gob.Decoder, encoder *gob.Encoder) {
	switch command {
	case "RegisterSensingData":
		format := &Format{FormType: "RegisterSensingData"}
		if err := encoder.Encode(format); err != nil {
			message.MyError(err, "syncFormatClient > RegisterSensingData > encoder.Encode")
		}
	}
}

func convertID(id string, pos int) string {
	id_int := new(big.Int)

	_, ok := id_int.SetString(id, 10)
	if !ok {
		message.MyMessage("Failed to convert string to big.Int")
	}

	mask := new(big.Int).Lsh(big.NewInt(1), uint(pos))
	id_int.Xor(id_int, mask)
	return id_int.String()
}

func getGID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}

func loadEnv() {
	// .envファイルの読み込み
	if err := godotenv.Load(os.Getenv("HOME") + "/.env"); err != nil {
		message.MyError(err, "loadEnv > godotenv.Load")
	}
	mes := os.Getenv("SAMPLE_MESSAGE")
	// fmt.Printf("\x1b[32m%v\x1b[0m\n", message)
	message.MyMessage(mes)
}
