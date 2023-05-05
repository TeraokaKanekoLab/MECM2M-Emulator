package main

import (
	"bytes"
	"context"
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
	"sync"
	"time"
)

const (
	protocol = "unix"
	layout   = "2006-01-02 15:04:05"
	timeSock = "/tmp/mecm2m/time.sock"
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
	//configファイルを読み込んで，センサデータ送信用のデータ，センサデータ登録用のデータを読み込む
	//PSNodeをいくつか用意しておく
	var socketFiles []string
	socketFiles = append(socketFiles, "/tmp/mecm2m/psnode_1_0001.sock", "/tmp/mecm2m/psnode_1_0002.sock", "/tmp/mecm2m/psnode_1_0003.sock")
	gids := make(chan uint64, len(socketFiles))
	cleanup(socketFiles...)

	quit := make(chan os.Signal)
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
				go registerSensingData(t)
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
				go psnode(conn, gid)
			}
		}(file, &wg)
	}
	wg.Wait()
	cleanup(timeSock)
	defer close(gids)
}

//mainプロセスからの時刻配布を受信・所定の一定時間間隔でSensingDBにセンサデータ登録
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
				m := &message.MyTime{}
				if err := decoderTime.Decode(m); err != nil {
					if err == io.EOF {
						break
					}
					message.MyError(err, "timeSync > decoderTime.Decode")
					break
				}

				getTimeContext := context.WithValue(mainContext, currentTime, m.CurrentTime)
				var t time.Time
				switch getTimeContext.Value(currentTime).(type) {
				case time.Time:
					m.Ack = true
					t = getTimeContext.Value(currentTime).(time.Time)
				default:
					m.Ack = false
				}
				if err := encoderTime.Encode(m); err != nil {
					message.MyError(err, "timeSync > encoderTime.Encode")
					break
				}
				retTime <- t
			}
		}(retTime)
	}
}

//センサデータ送信，センサデータ登録
func psnode(conn net.Conn, gid uint64) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	message.MyMessage("[MESSAGE] Call PSNode thread(" + strconv.FormatUint(gid, 10) + ")")

	for {
		//VSNodeとやりとりをする初めに型の同期をとる
		switch m := syncFormatServer(decoder, encoder); m.(type) {
		case *m2mapi.ResolveCurrentNode:
			format := m.(*m2mapi.ResolveCurrentNode)
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
			output := m2mapi.ResolveCurrentNode{}
			output.VNodeID_n = "PNodeID0001"
			output.Values = m2mapi.Value{Capability: "AAA", Time: currentTime.Format(layout), Value: 22.22}
			//センサデータをVSNodeに送信する
			if err := encoder.Encode(&output); err != nil {
				message.MyError(err, "vsnode > CurrentNode > encoder.Encode")
				break
			}
			message.MyWriteMessage(output)
		}
	}

}

//センサデータの登録
//PSNode -> VPoint -> VSNode -> SensingDB
func registerSensingData(t time.Time) {
	//センサデータ登録用の型を指定
	m := &m2mapi.DataForRegist{
		PNodeID:    "TESTPNodeID",
		Capability: "TESTCapability",
		Timestamp:  "TESTTimestamp",
		Value:      "TESTValue",
		PSinkID:    "TESTPSinkID",
		ServerID:   "TESTServerID",
		Lat:        "TESTLat",
		Lon:        "TESTLon",
		VNodeID:    "TESTVNodeID",
		VPointID:   "TESTVPointID",
	}

	//VPointへ接続
	//ソケットファイルはVPointのものを使用する（/tmp/mecm2m/vpoint_1_0001.sock）
	connDB, err := net.Dial(protocol, "/tmp/mecm2m/vpoint_1_0001.sock")
	if err != nil {
		message.MyError(err, "registerSensingDB > net.Dial")
	}
	decoderDB := gob.NewDecoder(connDB)
	encoderDB := gob.NewEncoder(connDB)

	syncFormatClient("RegisterSensingData", decoderDB, encoderDB)

	if err := encoderDB.Encode(m); err != nil {
		message.MyError(err, "registerSensingData > encoderDB.Encode")
	}
	fmt.Println("Data Register at...", t)
	//message.MyWriteMessage(m)
}

//VSNodeと型同期をするための関数
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
	case "CurrentNode", "CurrentPoint":
		typeM = &m2mapi.ResolveCurrentNode{}
	}
	return typeM
}

//SensingDBと型同期をするための関数
func syncFormatClient(command string, decoder *gob.Decoder, encoder *gob.Encoder) {
	switch command {
	case "RegisterSensingData":
		m := &Format{FormType: "RegisterSensingData"}
		if err := encoder.Encode(m); err != nil {
			message.MyError(err, "syncFormatClient > RegisterSensingData > encoder.Encode")
		}
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
