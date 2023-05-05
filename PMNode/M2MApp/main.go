package main

import (
	"bytes"
	"encoding/csv"
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"

	"mecm2m-Simulator/pkg/m2mapi"
	"mecm2m-Simulator/pkg/m2mapp"
	"mecm2m-Simulator/pkg/message"
)

const (
	protocol = "unix"
)

type Format struct {
	FormType string
}

// ソケットファイルの削除
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
	var socketFiles []string
	socketFiles = append(socketFiles, "/tmp/mecm2m/m2mapp_1.sock", "/tmp/sock1.sock")
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

	// goroutineの同期
	var wg sync.WaitGroup
	// wg.Done()が呼び出されるたびに()内の値がデクリメント
	wg.Add(3)
	for _, file := range socketFiles {
		go initialize(file, gids, &wg)
		data := <-gids
		fmt.Printf("GOROUTINE ID (%s): %d\n", file, data)
	}
	// wgが0になったら終了
	wg.Wait()
	defer close(gids)
}

func initialize(file string, gids chan uint64, wg *sync.WaitGroup) {
	gids <- getGID()
	gid := getGID()
	listener, err := net.Listen(protocol, file)
	if err != nil {
		message.MyError(err, "initialize > net.Listen")
	}
	s := "> [Initialize] Socket file launched: " + file
	message.MyMessage(s)
	for {
		conn, err := listener.Accept()
		if err != nil {
			message.MyError(err, "initialize > listener.Accept")
			break
		}

		switch file {
		case "/tmp/mecm2m/m2mapp_1.sock":
			go m2mApp(conn, gid)
			wg.Done()
		}
	}
}

// Appの起動をMainプロセスに示す
func m2mApp(conn net.Conn, gid uint64) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	message.MyMessage("[MESSAGE] Call M2M App thread")

	for {
		m := &m2mapp.App{}

		if err := decoder.Decode(m); err != nil {
			if err == io.EOF {
				message.MyMessage("<=== Closed By Client")
				break
			}
			message.MyError(err, "m2mApp > decoder.Decode")
			break
		}
		message.MyReadMessage(m)

		// AppIDが存在していれば，App起動を許可する
		if m.AppID != "" {
			m.Description = "OK"
			m.GID = gid
		} else {
			m.Description = "NG"
		}

		if err := encoder.Encode(m); err != nil {
			message.MyError(err, "m2mApp > encoder.Encode")
			break
		}
		message.MyWriteMessage(m)

		if string(m.Description) == "OK" {
			ExecuteApp(gid)
			break
		}
	}
}

// 実際にAppを動かす（センサデータ取得やアクチュエート）
func ExecuteApp(gid uint64) {
	for {
		var command string
		fmt.Printf("M2M App [GID:%d] > ", gid)
		fmt.Scan(&command)
		message.MyExit(command)
		options := loadInput(command)

		sockAddr := selectSocketFile(command)
		//fmt.Println(sockAddr)
		conn, err := net.Dial(protocol, sockAddr)
		if err != nil {
			message.MyError(err, "ExecuteApp > net.Dial")
		}
		//defer conn.Close()

		decoder := gob.NewDecoder(conn)
		encoder := gob.NewEncoder(conn)
		// commandExecutionする前に，Server側と型の同期を取りたい
		syncFormatClient(command, decoder, encoder)
		commandExecution(command, decoder, encoder, options)
	}
}

// 呼び出すAPIに応じて必要な入力をcsvファイルなどで用意しておき，それを読み込む
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

// M2M Appを実行する際のサーバ側のソケットファイルの選択
func selectSocketFile(command string) string {
	var sockAddr string
	defaultAddr := "/tmp/mecm2m"
	defaultExt := ".sock"
	switch command {
	case "m2mapi":
		sockAddr = defaultAddr + "/svr_1_m2mapi" + defaultExt
	case "point":
		sockAddr = defaultAddr + "/svr_1_m2mapi" + defaultExt
	case "node":
		sockAddr = defaultAddr + "/svr_1_m2mapi" + defaultExt
	case "past_node":
		sockAddr = defaultAddr + "/svr_1_m2mapi" + defaultExt
	default:
		sockAddr = defaultAddr + defaultExt
	}
	return sockAddr
}

func syncFormatClient(command string, decoder *gob.Decoder, encoder *gob.Encoder) {
	switch command {
	case "point":
		m := &Format{FormType: "Point"}
		if err := encoder.Encode(m); err != nil {
			message.MyError(err, "syncFormatClient > point > encoder.Encode")
		}

		if err := decoder.Decode(m); err != nil {
			message.MyError(err, "syncFormatClient > point > decoder.Decode")
		}
	case "node":
		m := &Format{FormType: "Node"}
		if err := encoder.Encode(m); err != nil {
			message.MyError(err, "syncFormatClient > node > encoder.Encode")
		}

		if err := decoder.Decode(m); err != nil {
			message.MyError(err, "syncFormatClient > node > decoder.Decode")
		}
	case "past_node":
		m := &Format{FormType: "PastNode"}
		if err := encoder.Encode(m); err != nil {
			message.MyError(err, "syncFormatClient > past_node > encoder.Encode")
		}

		if err := decoder.Decode(m); err != nil {
			message.MyError(err, "syncFormatClient > past_node > decoder.Decode")
		}
	}
}

// M2M Appで入力されたコマンド (e.g., point, node) に応じて実行
func commandExecution(command string, decoder *gob.Decoder, encoder *gob.Encoder, options []string) {
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
			message.MyError(err, "commandExecution > point > encoder.Encode")
		}
		message.MyWriteMessage(m)

		//ポイント解決の結果を受信する (PsinkのVPointID_n，Address)
		ms := []m2mapi.ResolvePoint{}
		if err := decoder.Decode(&ms); err != nil {
			message.MyError(err, "commandExecution > point > decoder.Decode")
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
			message.MyError(err, "commandExecution > node > encoder.Encode")
		}
		message.MyWriteMessage(m)

		//ノード解決の結果を受信する（PNodeのVNodeID_n, Cap）
		ms := []m2mapi.ResolveNode{}
		if err := decoder.Decode(&ms); err != nil {
			message.MyError(err, "commandExecution > node > decoder.Decode")
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
			message.MyError(err, "commandExecution > past_node > encoder.Encode")
		}
		message.MyWriteMessage(m)

		//ノードの過去データ解決を受信する（Value, Cap, Time）
		if err := decoder.Decode(m); err != nil {
			message.MyError(err, "commandExecution > past_node > decoder.Decode")
		}
		message.MyReadMessage(m)
	}
}

func getGID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	//fmt.Println(string(b))
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}
