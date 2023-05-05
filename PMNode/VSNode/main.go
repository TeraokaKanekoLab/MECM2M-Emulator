package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"mecm2m-Simulator/pkg/m2mapi"
	"mecm2m-Simulator/pkg/message"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
)

const (
	protocol          = "unix"
	sensingDBSockAddr = "/tmp/mecm2m/svr_1_sensingdb.sock"
)

type Format struct {
	FormType string
}

//VPointからのデータ通知で来たデータを充足条件データ取得でも使うためにバッファを用意する
var bufferSensorData m2mapi.DataForRegist

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
	//VSNodeをいくつか用意しておく
	var socketFiles []string
	socketFiles = append(socketFiles, "/tmp/mecm2m/vsnode_1_0001.sock", "/tmp/mecm2m/vsnode_1_0002.sock", "/tmp/mecm2m/vsnode_1_0003.sock")
	gids := make(chan uint64, len(socketFiles))
	cleanup(socketFiles...)

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)
	go func() {
		<-quit
		fmt.Println("ctrl-c pressed!")
		close(quit)
		cleanup(socketFiles...)
		os.Exit(0)
	}()

	for _, file := range socketFiles {
		go initialize(file, gids)
		data := <-gids
		fmt.Printf("GOROUTINE ID (%s): %d\n", file, data)
	}
	fmt.Scanln()
	defer close(gids)
}

func initialize(file string, gids chan uint64) {
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
		go vsnode(conn, gid)
	}
}

//過去データ取得，現在データ取得，充足条件データ取得
//2023/03/28 context.Background()を引数に入れてみる
func vsnode(conn net.Conn, gid uint64) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	message.MyMessage("[MESSAGE] Call VSNode thread(" + strconv.FormatUint(gid, 10) + ")")
LOOP:
	for {
		//型同期をして，型の種類に応じてスイッチ
		switch m := syncFormatServer(decoder, encoder); m.(type) {
		case *m2mapi.ResolvePastNode:
			format := m.(*m2mapi.ResolvePastNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "vsnode > PastNode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			//SensingDBへ接続
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

			//SensingDB()によるDB検索

			//受信する型はResolvePastNode
			m := m2mapi.ResolvePastNode{}
			if err := decoderDB.Decode(&m); err != nil {
				message.MyError(err, "vsnode > PastNode > decoderDB.Decode")
			}
			message.MyReadMessage(m)

			//DB検索結果をM2M APIに送信する
			if err := encoder.Encode(&m); err != nil {
				message.MyError(err, "vsnode > PastNode > encoder.Encode")
				break
			}
			message.MyWriteMessage(m)
		case *m2mapi.ResolveCurrentNode:
			format := m.(*m2mapi.ResolveCurrentNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "vsnode > CurrentNode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			//PSNodeとのやりとり
			//どのPSNodeスレッドとやりとりするかを判別できるような仕組みが必要だが，現状はpsnode_1_1.sock
			connPS, err := net.Dial(protocol, "/tmp/mecm2m/psnode_1_0001.sock")
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

			//PSNodeのセンサデータ送信を受ける

			//受信する型はResolveCurrentNode
			m := m2mapi.ResolveCurrentNode{}
			if err := decoderPS.Decode(&m); err != nil {
				message.MyError(err, "vsnode > CurrentNode > decoderPS.Decode")
			}
			message.MyReadMessage(m)

			//結果をM2M APIに送信する
			if err := encoder.Encode(&m); err != nil {
				message.MyError(err, "vsnode > CurrentNode > encoder.Encode")
				break
			}
			message.MyWriteMessage(m)
		case *m2mapi.ResolveConditionNode:
			format := m.(*m2mapi.ResolveConditionNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "vsnode > ConditionNode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			//VPointからのセンサデータ通知を受ける
			fmt.Println("before")
			//data := <-bufferSensorData
			data := bufferSensorData
			fmt.Println("after")
			val, _ := strconv.ParseFloat(data.Value, 64)

			//formatで受けた条件とdataを比較し，該当するデータであればM2M APIへ返す
			lowerLimit := format.Limit.LowerLimit
			upperLimit := format.Limit.UpperLimit
			//timeout := format.Timeout
			//確認
			//timeoutContext, cancelFunc := context.WithTimeout(alarm, timeout)
			if val >= lowerLimit && val < upperLimit {
				//送信型はDataforRegist
				if err := encoder.Encode(&data); err != nil {
					message.MyError(err, "vsnode > ConditionNode > encoder.Encode")
					break
				}
				message.MyWriteMessage(data)
			}
		case *m2mapi.DataForRegist:
			format := m.(*m2mapi.DataForRegist)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "vsnode > RegisterSensingData > decoder.Decode")
				break
			}
			message.MyMessage("Notification for Data Register")
			//message.MyReadMessage(*format)

			//バッファにデータ登録
			bufferSensorData = *format
			fmt.Println("data bufferd: ", bufferSensorData)

			//SensingDBへ接続
			connDB, err := net.Dial(protocol, sensingDBSockAddr)
			if err != nil {
				message.MyError(err, "vsnode > RegisterSensingData > net.Dial")
			}
			decoderDB := gob.NewDecoder(connDB)
			encoderDB := gob.NewEncoder(connDB)

			//SensingDBとの型同期
			syncFormatClient("RegisterSensingData", decoderDB, encoderDB)

			//SensingDBへのデータ通知
			if err := encoderDB.Encode(format); err != nil {
				message.MyError(err, "vsnode > RegisterSensingData > encoderVS.Encode")
			}
			//message.MyWriteMessage(*format)
		default:
			fmt.Println("no match. GID: ", gid)
			break LOOP
		}
	}
}

//M2M APIと型同期をするための関数
func syncFormatServer(decoder *gob.Decoder, encoder *gob.Encoder) any {
	m := &Format{}
	if err := decoder.Decode(m); err != nil {
		if err == io.EOF {
			typeM := "exit"
			return typeM
		} else {
			message.MyError(err, "syncFormatServer > decoder.Decode")
		}
	}
	typeResult := m.FormType

	var typeM any
	switch typeResult {
	case "PastNode":
		typeM = &m2mapi.ResolvePastNode{}
	case "CurrentNode":
		typeM = &m2mapi.ResolveCurrentNode{}
	case "ConditionNode":
		typeM = &m2mapi.ResolveConditionNode{}
	case "RegisterSensingData":
		typeM = &m2mapi.DataForRegist{}
	}
	return typeM
}

//SensingDB, PSNode, PMNodeと型同期をするための関数
func syncFormatClient(command string, decoder *gob.Decoder, encoder *gob.Encoder) {
	m := &Format{}
	switch command {
	case "PastNode":
		m.FormType = "PastNode"
	case "CurrentNode":
		m.FormType = "CurrentNode"
	case "ConditionNode":
		m.FormType = "ConditionNode"
	case "RegisterSensingData":
		m.FormType = "RegisterSensingData"
	}
	if err := encoder.Encode(m); err != nil {
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
