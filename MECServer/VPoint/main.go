package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mecm2m-Simulator/pkg/m2mapi"
	"mecm2m-Simulator/pkg/message"
	"mecm2m-Simulator/pkg/vpoint"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

const (
	protocol            = "unix"
	socket_address_root = "/tmp/mecm2m/"
)

type Format struct {
	FormType string
}

var graphDBSockAddr string
var sensingDBSockAddr string
var server_num string

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
	// コマンドライン引数にソケットファイル群をまとめたファイルをしていして，初めにそのファイルを読み込む
	if len(os.Args) != 2 {
		fmt.Println("There is no socket files")
		os.Exit(1)
	}
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
	socket_file_name := os.Args[1]
	data, err := ioutil.ReadFile(socket_file_name)
	if err != nil {
		message.MyError(err, "Failed to read socket file")
	}

	// このVPointが所属するサーバ番号をArgs[1]から取り出す
	server_num_first_index := strings.LastIndex(socket_file_name, "_")
	server_num_last_index := strings.LastIndex(socket_file_name, ".")
	server_num = socket_file_name[server_num_first_index+1 : server_num_last_index]
	graphDBSockAddr = socket_address_root + "svr_" + server_num + "_graphdb.sock"
	sensingDBSockAddr = socket_address_root + "svr_" + server_num + "_sensingdb.sock"

	var socket_files vpoint.VPointSocketFiles

	if err := json.Unmarshal(data, &socket_files); err != nil {
		message.MyError(err, "Failed to unmarshal json")
	}

	//VPointをいくつか用意しておく
	var socketFiles []string
	socketFiles = append(socketFiles, socket_files.VPoints...)
	gids := make(chan uint64, len(socketFiles))
	cleanup(socketFiles...)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	go func() {
		<-quit
		fmt.Println("ctrl-c pressed!")
		close(quit)
		cleanup(socketFiles...)
		os.Exit(0)
	}()

	var wg sync.WaitGroup

	for _, file := range socketFiles {
		wg.Add(1)
		go initialize(file, gids, &wg)
		data := <-gids
		fmt.Printf("GOROUTINE ID (%s): %d\n", file, data)
	}
	wg.Wait()
	defer close(gids)
}

func initialize(file string, gids chan uint64, wg *sync.WaitGroup) {
	defer wg.Done()
	gids <- getGID()
	gid := getGID()
	listener, err := net.Listen(protocol, file)
	if err != nil {
		message.MyError(err, "initialize > net.Listen")
	}
	message.MyMessage("> [Initialize] Socket file launched: " + file)
	for {
		conn, err := listener.Accept()
		if err != nil {
			message.MyError(err, "initialize > listener.Accept")
			break
		}
		go vpoints(conn, gid)
	}
}

// 過去データ取得，現在データ取得，充足条件データ取得
func vpoints(conn net.Conn, gid uint64) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	message.MyMessage("[MESSAGE] Call VPoint thread(" + strconv.FormatUint(gid, 10) + ")")

LOOP:
	for {
		//型同期をして，型の種類に応じてスイッチ
		switch vpointsCommand := syncFormatServer(decoder, encoder); vpointsCommand.(type) {
		case *m2mapi.ResolvePastPoint:
			format := vpointsCommand.(*m2mapi.ResolvePastPoint)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "vpoint > PastPoint > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			//SensingDBへ接続
			connDB, err := net.Dial(protocol, sensingDBSockAddr)
			if err != nil {
				message.MyError(err, "vpoint > PastPoint > net.Dial")
			}
			decoderDB := gob.NewDecoder(connDB)
			encoderDB := gob.NewEncoder(connDB)

			syncFormatClient("PastPoint", decoderDB, encoderDB)

			if err := encoderDB.Encode(format); err != nil {
				message.MyError(err, "vpoint > PastPoint > encoderDB.Encode")
			}
			message.MyWriteMessage(*format)

			//SensingDB()によるDB検索

			//受信する型はResolvePastPoint
			past_point_output := m2mapi.ResolvePastPoint{}
			if err := decoderDB.Decode(&past_point_output); err != nil {
				message.MyError(err, "vpoint > PastPoint > decoderDB.Decode")
			}
			message.MyReadMessage(past_point_output)

			//DB検索結果をM2M APIに送信する
			if err := encoder.Encode(&past_point_output); err != nil {
				message.MyError(err, "vpoint > PastPoint > encoder.Encode")
				break
			}
			message.MyWriteMessage(past_point_output)
		case *m2mapi.ResolveCurrentPoint:
			format := vpointsCommand.(*m2mapi.ResolveCurrentPoint)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "vpoint > CurrentPoint > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// パケットフォーマット内のVPointIDを元に，そこに接続するVNodeをLocal GraphDBで検索する
			connDB, err := net.Dial(protocol, graphDBSockAddr)
			if err != nil {
				message.MyError(err, "vpoint > CurrentPoint > ResolveVNode > net.Dial")
			}
			decoderDB := gob.NewDecoder(connDB)
			encoderDB := gob.NewEncoder(connDB)

			syncFormatClient("CurrentPointVNode", decoderDB, encoderDB)

			vpoint_id := vpoint.CurrentPointVNode{
				VPointID:   format.VPointID_n,
				Capability: format.Capability,
			}
			if err := encoderDB.Encode(&vpoint_id); err != nil {
				message.MyError(err, "vpoint > CurrentPoint > encoderDB.Encode")
			}

			// Local GraphDB でのVNode検索

			// 受信する型はCurrentPointVNode
			vnode_socket_addresses := vpoint.CurrentPointVNode{}
			if err := decoderDB.Decode(&vnode_socket_addresses); err != nil {
				message.MyError(err, "vpoint > CurrentPoint > decoderDB.Decode")
			}

			//複数のVNodeに接続
			var connectedVNodeSockAddr []string
			var connectedVNodeID []string
			connectedVNodeSockAddr = append(connectedVNodeSockAddr, vnode_socket_addresses.VNodeSockAddr...)
			connectedVNodeID = append(connectedVNodeID, vnode_socket_addresses.VNodeID...)
			current_point_ch := make(chan *m2mapi.ResolveCurrentPoint)
			go func() {
				current_point_aggregate := m2mapi.ResolveCurrentPoint{}
				for index, vnodeSockAddr := range connectedVNodeSockAddr {
					connVS, err := net.Dial(protocol, vnodeSockAddr)
					if err != nil {
						message.MyError(err, "vpoint > CurrentPoint > net.Dial")
					}
					decoderVS := gob.NewDecoder(connVS)
					encoderVS := gob.NewEncoder(connVS)

					syncFormatClient("CurrentNode", decoderVS, encoderVS)

					vpoint_info_to_vnode := m2mapi.ResolveCurrentNode{
						VNodeID_n:  connectedVNodeID[index],
						Capability: format.Capability,
					}
					if err := encoderVS.Encode(&vpoint_info_to_vnode); err != nil {
						message.MyError(err, "vpoint > CurrentPoint > encoderVS.Encode")
					}
					message.MyWriteMessage(vpoint_info_to_vnode)

					// VSNodeからセンサデータを受ける

					// 受信する型はResolveCurrentNode
					sensing_data_from_vnode := m2mapi.ResolveCurrentNode{}
					if err := decoderVS.Decode(&sensing_data_from_vnode); err != nil {
						message.MyError(err, "vpoint > CurrentPoint > decoderVS.Decode")
					}
					message.MyReadMessage(sensing_data_from_vnode)
					data := m2mapi.SensorData{
						VNodeID_n: sensing_data_from_vnode.VNodeID_n,
					}
					value := m2mapi.Value{
						Capability: sensing_data_from_vnode.Values.Capability,
						Time:       sensing_data_from_vnode.Values.Time,
						Value:      sensing_data_from_vnode.Values.Value,
					}
					data.Values = append(data.Values, value)
					current_point_aggregate.Datas = append(current_point_aggregate.Datas, data)
				}
				current_point_ch <- &current_point_aggregate
			}()
			//結果をM2M APIに送信する
			current_point_output := <-current_point_ch
			if err := encoder.Encode(current_point_output); err != nil {
				message.MyError(err, "vpoint > CurrentPoint > encoder.Encode")
				break
			}
			message.MyWriteMessage(*current_point_output)
		default:
			fmt.Println("no match. GID: ", gid)
			break LOOP
		}
	}
}

// M2M APIと型同期をするための関数
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
	case "PastPoint":
		typeM = &m2mapi.ResolvePastPoint{}
	case "CurrentPoint":
		typeM = &m2mapi.ResolveCurrentPoint{}
	}
	return typeM
}

// SensingDB, PSNode, PMNodeと型同期をするための関数
func syncFormatClient(command string, decoder *gob.Decoder, encoder *gob.Encoder) {
	format := &Format{}
	switch command {
	case "PastPoint":
		format.FormType = "PastPoint"
	case "CurrentPoint":
		format.FormType = "CurrentPoint"
	case "CurrentNode":
		format.FormType = "CurrentNode"
	case "CurrentPointVNode":
		format.FormType = "CurrentPointVNode"
	}
	if err := encoder.Encode(format); err != nil {
		message.MyError(err, "syncFormatClient > "+command+" > encoder.Encode")
	}
}

func getGID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}
