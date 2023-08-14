package main

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"mecm2m-Emulator/pkg/m2mapi"
	"mecm2m-Emulator/pkg/message"
	"mecm2m-Emulator/pkg/server"
	"mecm2m-Emulator/pkg/vsnode"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

const (
	protocol                 = "unix"
	socket_address_root      = "/tmp/mecm2m/"
	dataResisterSock         = "/tmp/mecm2m/data_resister.sock"
	link_socket_address_root = "/tmp/mecm2m/link-process/"
)

type Format struct {
	FormType string
}

// VPointからのデータ通知で来たデータを充足条件データ取得でも使うためにバッファを用意する
var bufferSensorData = make(map[string]m2mapi.DataForRegist)

// PSNode のセッションキーのキャッシュ群
var psnode_session_keys = make([]string, 3)
var mu sync.Mutex
var graphDBSockAddr string
var sensingDBSockAddr string
var server_num string
var data_resister_socket string

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

	// このVSNodeが所属するサーバ番号をArgs[1]から取り出す
	server_num_first_index := strings.LastIndex(socket_file_name, "_")
	server_num_last_index := strings.LastIndex(socket_file_name, ".")
	server_num = socket_file_name[server_num_first_index+1 : server_num_last_index]
	graphDBSockAddr = socket_address_root + "svr_" + server_num + "_graphdb.sock"
	sensingDBSockAddr = socket_address_root + "svr_" + server_num + "_sensingdb.sock"

	data_resister_socket = "/tmp/mecm2m/data_resister_" + server_num + ".sock"

	var socket_files vsnode.VSNodeSocketFiles

	if err := json.Unmarshal(data, &socket_files); err != nil {
		message.MyError(err, "Failed to unmarshal json")
	}

	// VSNodeをいくつか用意しておく
	var socketFiles []string
	socketFiles = append(socketFiles, socket_files.VSNodes...)
	for i := 0; i < len(socketFiles); i++ {
		var newData m2mapi.DataForRegist
		switch i {
		case 0:
			bufferSensorData["MaxTemp"] = newData
		case 1:
			bufferSensorData["MaxHumid"] = newData
		case 2:
			bufferSensorData["MaxWind"] = newData
		}
	}
	socketFiles = append(socketFiles, data_resister_socket)
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
		if file == data_resister_socket {
			go resisterSensingData(conn)
		} else {
			go vsnodes(conn, gid)
		}
	}
}

// 過去データ取得，現在データ取得，充足条件データ取得
// 2023/03/28 context.Background()を引数に入れてみる
func vsnodes(conn net.Conn, gid uint64) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	message.MyMessage("[MESSAGE] Call VSNode thread(" + strconv.FormatUint(gid, 10) + ")")
LOOP:
	for {
		// 型同期をして，型の種類に応じてスイッチ
		//mu.Lock()
		//defer mu.Unlock()
		switch vsnodesCommand := syncFormatServer(decoder, encoder); vsnodesCommand.(type) {
		case *m2mapi.ResolvePastNode:
			format := vsnodesCommand.(*m2mapi.ResolvePastNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "vsnode > PastNode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// SensingDBへ接続
			connDB, err := net.Dial(protocol, sensingDBSockAddr)
			if err != nil {
				message.MyError(err, "vsnode > PastNode > net.Dial")
			}
			decoderDB := gob.NewDecoder(connDB)
			encoderDB := gob.NewEncoder(connDB)

			syncFormatClient("PastNode", decoderDB, encoderDB)

			if err := encoderDB.Encode(format); err != nil {
				message.MyError(err, "vsnode > PastNode > encoderDB.Encode")
			}
			message.MyWriteMessage(*format)

			// SensingDB()によるDB検索

			// 受信する型はResolvePastNode
			past_node_output := m2mapi.ResolvePastNode{}
			if err := decoderDB.Decode(&past_node_output); err != nil {
				message.MyError(err, "vsnode > PastNode > decoderDB.Decode")
			}
			message.MyReadMessage(past_node_output)

			// DB検索結果をM2M APIに送信する
			if err := encoder.Encode(&past_node_output); err != nil {
				message.MyError(err, "vsnode > PastNode > encoder.Encode")
				break
			}
			message.MyWriteMessage(past_node_output)
		case *m2mapi.ResolveCurrentNode:
			format := vsnodesCommand.(*m2mapi.ResolveCurrentNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "vsnode > CurrentNode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// PSNodeとのやりとり
			// 閉域網リンクプロセスを通って，RTTを付与
			psnode_id := convertID(format.VNodeID_n, 63)
			psnode_socket := socket_address_root + "psnode_" + server_num + "_" + psnode_id + ".sock"
			format.DestSocketAddr = psnode_socket
			linkSrcAddr := link_socket_address_root + "access-network_" + format.VNodeID_n + "_" + psnode_id + ".sock"

			// リンクプロセスへ転送
			connPS, err := net.Dial(protocol, linkSrcAddr)
			if err != nil {
				message.MyError(err, "vsnode > CurrentNode > net.Dial")
			}
			decoderPS := gob.NewDecoder(connPS)
			encoderPS := gob.NewEncoder(connPS)

			syncFormatClient("CurrentNode", decoderPS, encoderPS)

			if err := encoderPS.Encode(format); err != nil {
				message.MyError(err, "vsnode > CurrentNode > encoderPS.Encode")
			}
			message.MyWriteMessage(*format)

			// PSNodeのセンサデータ送信を受ける

			// 受信する型はResolveCurrentNode
			current_node_output := m2mapi.ResolveCurrentNode{}
			if err := decoderPS.Decode(&current_node_output); err != nil {
				message.MyError(err, "vsnode > CurrentNode > decoderPS.Decode")
			}
			message.MyReadMessage(current_node_output)

			// 結果を送信する
			if err := encoder.Encode(&current_node_output); err != nil {
				message.MyError(err, "vsnode > CurrentNode > encoder.Encode")
				break
			}
			message.MyWriteMessage(current_node_output)
		case *m2mapi.ResolveConditionNode:
			format := vsnodesCommand.(*m2mapi.ResolveConditionNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "vsnode > ConditionNode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			inputCapability := format.Capability
			data := bufferSensorData[inputCapability]
			val := data.Value

			// formatで受けた条件とdataを比較し，該当するデータであればM2M APIへ返す
			lowerLimit := format.Limit.LowerLimit
			upperLimit := format.Limit.UpperLimit
			timeout := format.Timeout

			timeoutContext, cancelFunc := context.WithTimeout(context.Background(), timeout)
			defer cancelFunc()

			go func() {
				for {
					select {
					case <-timeoutContext.Done():
						fmt.Println("タイムアウト期限")
						nullData := m2mapi.DataForRegist{PNodeID: "NULL"}
						if err := encoder.Encode(&nullData); err != nil {
							var opErr *net.OpError
							if errors.As(err, &opErr) {
								if opErr.Op == "write" && strings.Contains(opErr.Err.Error(), "use of closed network connection") {
									fmt.Println("想定通り")
								} else {
									message.MyError(err, "vsnode > ConditionNodeTimeout > encoder.Encode")
								}
							}
						}
						return
					default:
						mu.Lock()
						if val != bufferSensorData[inputCapability].Value {
							// バッファデータ更新
							val = bufferSensorData[inputCapability].Value
						}
						mu.Unlock()
						//fmt.Println("2023-06-26: ", val)

						if val >= lowerLimit && val < upperLimit {
							//送信型はDataforRegist
							data = bufferSensorData[inputCapability]
							if err := encoder.Encode(&data); err != nil {
								message.MyError(err, "vsnode > ConditionNode > encoder.Encode")
								break
							}
							message.MyWriteMessage(data)
							bufferSensorData[inputCapability] = m2mapi.DataForRegist{}
						} else {
							continue
						}
					}
				}
			}()
		case *m2mapi.Actuate:
			format := vsnodesCommand.(*m2mapi.Actuate)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "vsnode > Actuate > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// PSNodeとのやりとり
			// 閉域網リンクプロセスを通って，RTTを付与
			psnode_id := convertID(format.VNodeID_n, 63)
			psnode_socket := socket_address_root + "psnode_" + server_num + "_" + psnode_id + ".sock"
			format.DestSocketAddr = psnode_socket
			linkSrcAddr := link_socket_address_root + "access-network_" + format.VNodeID_n + "_" + psnode_id + ".sock"

			// リンクプロセスへ転送
			connPS, err := net.Dial(protocol, linkSrcAddr)
			if err != nil {
				message.MyError(err, "vsnode > Actuate > net.Dial")
			}
			decoderPS := gob.NewDecoder(connPS)
			encoderPS := gob.NewEncoder(connPS)

			syncFormatClient("Actuate", decoderPS, encoderPS)

			if err := encoderPS.Encode(format); err != nil {
				message.MyError(err, "vsnode > Actuate > encoderPS.Encode")
			}
			message.MyWriteMessage(*format)

			// アクチュエータの状態結果を受ける

			// 受信する型はActuate
			actuate_output := m2mapi.Actuate{}
			if err := decoderPS.Decode(&actuate_output); err != nil {
				message.MyError(err, "vsnode > Actuate > decoderPS.Decode")
			}
			message.MyReadMessage(actuate_output)

			// 結果をVPointに送信する
			if err := encoder.Encode(&actuate_output); err != nil {
				message.MyError(err, "vsnode > Actuate > encoder.Encode")
				break
			}
			message.MyWriteMessage(actuate_output)
		default:
			fmt.Println("no match. GID: ", gid)
			break LOOP
		}
	}
}

// センサデータ登録用
func resisterSensingData(conn net.Conn) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	//message.MyMessage("[MESSAGE] Call resister sensing data")

	//LOOP:
	for {
		switch vsnodesCommand := syncFormatServer(decoder, encoder); vsnodesCommand.(type) {
		case *m2mapi.DataForRegist:
			format := vsnodesCommand.(*m2mapi.DataForRegist)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "vsnode > RegisterSensingData > decoder.Decode")
				break
			}
			//message.MyMessage("Notification for Data Register")
			//message.MyReadMessage(*format)

			// バッファにデータ登録
			mu.Lock()
			registerCapability := format.Capability
			bufferSensorData[registerCapability] = *format
			mu.Unlock()
			//fmt.Println("data bufferd: ", bufferSensorData[registerCapability])

			// PSNodeのセッションキーをキャッシュしていない場合，Local GraphDB にセッションキーを聞きに行く
			if psnode_session_keys[0] == "" {
				connSessionKey, err := net.Dial(protocol, graphDBSockAddr)
				if err != nil {
					message.MyError(err, "vsnode > RegisterSensingData > SessionKey > net.Dial")
				}
				decoderSessionKey := gob.NewDecoder(connSessionKey)
				encoderSessionKey := gob.NewEncoder(connSessionKey)

				// GraphDBとの型同期
				syncFormatClient("SessionKey", decoderSessionKey, encoderSessionKey)

				// GraphDBへセッションキーリクエスト
				session_key_request := &server.RequestSessionKey{
					PNodeID: format.PNodeID,
				}
				if err := encoderSessionKey.Encode(session_key_request); err != nil {
					message.MyError(err, "vsnode > RegisterSensingData > encoderSessionKey.Encode")
				}

				// Local GraphDBでのセッションキーの検索

				// Local GraphDB からセッションキーを受け取る
				ms := server.RequestSessionKey{}
				if err := decoderSessionKey.Decode(&ms); err != nil {
					message.MyError(err, "vsnode > RegisterSensingData > decoderSessionKey.Decode")
				}

				// キャッシュ情報に登録
				mu.Lock()
				psnode_session_keys[0] = ms.SessionKey
				mu.Unlock()
			}

			// Local SensingDBへ接続
			connDBLocal, err := net.Dial(protocol, sensingDBSockAddr)
			if err != nil {
				message.MyError(err, "vsnode > RegisterSensingData > ConnectionLocal > net.Dial")
			}
			decoderDBLocal := gob.NewDecoder(connDBLocal)
			encoderDBLocal := gob.NewEncoder(connDBLocal)

			// Local SensingDBとの型同期
			syncFormatClient("RegisterSensingData", decoderDBLocal, encoderDBLocal)

			// Local SensingDBへのデータ通知
			if err := encoderDBLocal.Encode(format); err != nil {
				message.MyError(err, "vsnode > RegisterSensingData > encoderDBLocal.Encode")
			}
			/*
				default:
					fmt.Println("break LOOP")
					break LOOP
			*/
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
	case "PastNode":
		typeM = &m2mapi.ResolvePastNode{}
	case "CurrentNode":
		typeM = &m2mapi.ResolveCurrentNode{}
	case "ConditionNode":
		typeM = &m2mapi.ResolveConditionNode{}
	case "Actuate":
		typeM = &m2mapi.Actuate{}
	case "RegisterSensingData":
		typeM = &m2mapi.DataForRegist{}
	}
	return typeM
}

// SensingDB, PSNode, PMNodeと型同期をするための関数
func syncFormatClient(command string, decoder *gob.Decoder, encoder *gob.Encoder) {
	m := &Format{}
	switch command {
	case "PastNode":
		m.FormType = "PastNode"
	case "CurrentNode":
		m.FormType = "CurrentNode"
	case "ConditionNode":
		m.FormType = "ConditionNode"
	case "Actuate":
		m.FormType = "Actuate"
	case "RegisterSensingData":
		m.FormType = "RegisterSensingData"
	case "SessionKey":
		m.FormType = "SessionKey"
	}
	if err := encoder.Encode(m); err != nil {
		message.MyError(err, "syncFormatClient > "+command+" > encoder.Encode")
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
