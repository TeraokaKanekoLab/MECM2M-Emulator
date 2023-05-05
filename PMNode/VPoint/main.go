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
	//VPointをいくつか用意しておく
	var socketFiles []string
	socketFiles = append(socketFiles, "/tmp/mecm2m/vpoint_1_0001.sock", "/tmp/mecm2m/vpoint_1_0002.sock", "/tmp/mecm2m/vpoint_1_0003.sock")
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
		go vpoint(conn, gid)
	}
}

//過去データ取得，現在データ取得，充足条件データ取得
func vpoint(conn net.Conn, gid uint64) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	message.MyMessage("[MESSAGE] Call VPoint thread(" + strconv.FormatUint(gid, 10) + ")")

LOOP:
	for {
		//型同期をして，型の種類に応じてスイッチ
		switch m := syncFormatServer(decoder, encoder); m.(type) {
		case *m2mapi.ResolvePastPoint:
			format := m.(*m2mapi.ResolvePastPoint)
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
			m := m2mapi.ResolvePastPoint{}
			if err := decoderDB.Decode(&m); err != nil {
				message.MyError(err, "vpoint > PastPoint > decoderDB.Decode")
			}
			message.MyReadMessage(m)

			//DB検索結果をM2M APIに送信する
			if err := encoder.Encode(&m); err != nil {
				message.MyError(err, "vpoint > PastPoint > encoder.Encode")
				break
			}
			message.MyWriteMessage(m)
		case *m2mapi.ResolveCurrentPoint:
			format := m.(*m2mapi.ResolveCurrentPoint)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "vpoint > CurrentPoint > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			//複数のPSNodeに接続
			//ここでは，psnode_1_1, psnode_1_2に接続するとする．
			var connectedPSNode []string
			connectedPSNode = append(connectedPSNode, "/tmp/mecm2m/psnode_1_0001.sock", "/tmp/mecm2m/psnode_1_0002.sock")
			ch := make(chan *m2mapi.ResolveCurrentPoint)
			go func() {
				ms := m2mapi.ResolveCurrentPoint{}
				for _, psnodeSockAddr := range connectedPSNode {
					connPS, err := net.Dial(protocol, psnodeSockAddr)
					if err != nil {
						message.MyError(err, "vpoint > CurrentPoint > net.Dial")
					}
					decoderPS := gob.NewDecoder(connPS)
					encoderPS := gob.NewEncoder(connPS)

					syncFormatClient("CurrentPoint", decoderPS, encoderPS)

					m := m2mapi.ResolveCurrentNode{
						Capability: format.Capability,
					}
					if err := encoderPS.Encode(&m); err != nil {
						message.MyError(err, "vpoint > CurrentPoint > encoderPS.Encode")
					}
					message.MyWriteMessage(m)

					//PSNodeのセンサデータ送信を受ける

					//受信する型はResolveCurrentNode
					if err := decoderPS.Decode(&m); err != nil {
						message.MyError(err, "vpoint > CurrentPoint > decoderPS.Decode")
					}
					message.MyReadMessage(m)
					data := m2mapi.SensorData{
						VNodeID_n: m.VNodeID_n,
					}
					value := m2mapi.Value{
						Capability: m.Values.Capability,
						Time:       m.Values.Time,
						Value:      m.Values.Value,
					}
					data.Values = append(data.Values, value)
					ms.Datas = append(ms.Datas, data)
				}
				ch <- &ms
			}()
			//結果をM2M APIに送信する
			msRec := <-ch
			if err := encoder.Encode(msRec); err != nil {
				message.MyError(err, "vpoint > CurrentPoint > encoder.Encode")
				break
			}
			message.MyWriteMessage(msRec)
		case *m2mapi.DataForRegist:
			format := m.(*m2mapi.DataForRegist)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "vpoint > RegisterSensingData > decoder.Decode")
				break
			}
			message.MyMessage("Notification for Data Register")
			//message.MyReadMessage(*format)

			//VSNodeへ接続
			connVS, err := net.Dial(protocol, "/tmp/mecm2m/vsnode_1_0001.sock")
			if err != nil {
				message.MyError(err, "vpoint > RegisterSensingData > net.Dial")
			}
			decoderVS := gob.NewDecoder(connVS)
			encoderVS := gob.NewEncoder(connVS)

			//VSNodeとの型同期
			syncFormatClient("RegisterSensingData", decoderVS, encoderVS)

			//VSNodeへのデータ通知
			if err := encoderVS.Encode(format); err != nil {
				message.MyError(err, "vpoint > RegisterSensingData > encoderVS.Encode")
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
	case "PastPoint":
		typeM = &m2mapi.ResolvePastPoint{}
	case "CurrentPoint":
		typeM = &m2mapi.ResolveCurrentPoint{}
	case "RegisterSensingData":
		typeM = &m2mapi.DataForRegist{}
	}
	return typeM
}

//SensingDB, PSNode, PMNodeと型同期をするための関数
func syncFormatClient(command string, decoder *gob.Decoder, encoder *gob.Encoder) {
	m := &Format{}
	switch command {
	case "PastPoint":
		m.FormType = "PastPoint"
	case "CurrentPoint":
		m.FormType = "CurrentPoint"
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
