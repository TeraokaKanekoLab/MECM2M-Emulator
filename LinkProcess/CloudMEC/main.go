package main

import (
	"encoding/csv"
	"encoding/gob"
	"fmt"
	"io"
	"mecm2m-Simulator/pkg/m2mapi"
	"mecm2m-Simulator/pkg/message"
	"mecm2m-Simulator/pkg/mserver"
	"mecm2m-Simulator/pkg/server"
	"mecm2m-Simulator/pkg/vpoint"
	"net"
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
	rtt_file                 = "rtt.csv"
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
	loadEnv()
	// .envファイルの EDGE_SERVER_NUM より，ソケットアドレスを作成
	var socketFiles []string
	edge_server_num, _ := strconv.Atoi(os.Getenv("EDGE_SERVER_NUM"))
	for i := 0; i < edge_server_num; i++ {
		for j := i + 1; j <= edge_server_num; j++ {
			i_str := strconv.Itoa(i)
			j_str := strconv.Itoa(j)
			link_socket_addr_src := link_socket_address_root + "internet_" + i_str + "_" + j_str + ".sock"
			link_socket_addr_dst := link_socket_address_root + "internet_" + j_str + "_" + i_str + ".sock"
			socketFiles = append(socketFiles, link_socket_addr_dst, link_socket_addr_src)
		}
	}
	cleanup(socketFiles...)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGALRM)
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
		go initialize(file, &wg)
	}

	wg.Wait()
	/*
		listener, err := net.Listen(protocol, "/tmp/mecm2m/a.sock")
		if err != nil {
			message.MyError(err, "main")
		}
		conn, err := listener.Accept()
		if err != nil {
			message.MyError(err, "main")
		}
		decoder := gob.NewDecoder(conn)
		encoder := gob.NewEncoder(conn)
		format := &m2mapi.ResolvePoint{}
		formType := &Format{}
		decoder.Decode(formType)
		decoder.Decode(format)

		fmt.Println(format)
		time.Sleep(5 * time.Second)

		format.VPointID_n = "YES"
		answer := []m2mapi.ResolvePoint{}
		answer = append(answer, *format)
		encoder.Encode(&answer)
	*/
}

func initialize(file string, wg *sync.WaitGroup) {
	defer wg.Done()
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

		go connectionLink(conn, file)
	}
}

func connectionLink(conn net.Conn, file string) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	message.MyMessage("[MESSAGE] Call Link Process thread")

	for {
		// 型同期をして，型の種類に応じてスイッチ
		switch m2mApiCommand := syncFormatServer(decoder, encoder); m2mApiCommand.(type) {
		case *m2mapi.ResolvePoint:
			format := m2mApiCommand.(*m2mapi.ResolvePoint)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "connectionLink > Point > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// 開いているソケットファイル名からsrc, dstのサーバを割り出す
			src_index := strings.Index(file, "_")
			dst_index := strings.LastIndex(file, "_")
			dot_index := strings.LastIndex(file, ".")
			src_server_num := file[src_index+1 : dst_index]
			dst_server_num := file[dst_index+1 : dot_index]

			// RTT時間の検索
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
				if (record[0] == src_server_num && record[1] == dst_server_num) || (record[1] == src_server_num && record[0] == dst_server_num) {
					rtt_float, _ := strconv.ParseFloat(record[2], 64)
					rtt_half_float := rtt_float / 2
					rtt_half_str := strconv.FormatFloat(rtt_half_float, 'f', 2, 64) + "s"
					rtt_half, _ = time.ParseDuration(rtt_half_str)
				}
			}

			// RTT/2 時間待機
			fmt.Println("sleep RTT/2 (upstream)")
			time.Sleep(rtt_half)

			// 宛先ソケットアドレス用の通信経路を確立
			dst_socket_address := format.DestSocketAddr

			connDst, err := net.Dial(protocol, dst_socket_address)
			if err != nil {
				message.MyError(err, "connectionLink > Point > net.Dial")
			}
			decoderDst := gob.NewDecoder(connDst)
			encoderDst := gob.NewEncoder(connDst)

			syncFormatClient("Point", decoderDst, encoderDst)

			if err := encoderDst.Encode(format); err != nil {
				message.MyError(err, "connectionLink > Point > encoderDst.Encode")
			}
			message.MyWriteMessage(*format)

			// Global GraphDB() によるDB検索

			// RTT/2 時間待機
			fmt.Println("sleep RTT/2 (downstream)")
			time.Sleep(rtt_half)

			// 受信する型は[]ResolvePoint
			point_output := []m2mapi.ResolvePoint{}
			if err := decoderDst.Decode(&point_output); err != nil {
				message.MyError(err, "connectionLink > Point > decoderDst.Decode")
			}

			// 送信元に結果を転送
			if err := encoder.Encode(&point_output); err != nil {
				message.MyError(err, "connectionLink > Point > encoder.Encode")
			}
			message.MyWriteMessage(point_output)
		}
	}
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
	case "Point":
		typeM = &m2mapi.ResolvePoint{}
	case "Node":
		typeM = &m2mapi.ResolveNode{}
	case "PastNode":
		typeM = &m2mapi.ResolvePastNode{}
	case "PastPoint":
		typeM = &m2mapi.ResolvePastPoint{}
	case "CurrentNode":
		typeM = &m2mapi.ResolveCurrentNode{}
	case "CurrentPoint":
		typeM = &m2mapi.ResolveCurrentPoint{}
	case "ConditionNode":
		typeM = &m2mapi.ResolveConditionNode{}
	case "ConditionPoint":
		typeM = &m2mapi.ResolveConditionPoint{}
	case "Actuate":
		typeM = &m2mapi.Actuate{}
	case "RegisterSensingData":
		typeM = &m2mapi.DataForRegist{}
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
	}
	return typeM
}

// 型同期をするための関数
func syncFormatClient(command string, decoder *gob.Decoder, encoder *gob.Encoder) {
	format := &Format{}
	switch command {
	case "Point":
		format.FormType = "Point"
	case "Node":
		format.FormType = "Node"
	case "PastNode":
		format.FormType = "PastNode"
	case "PastPoint":
		format.FormType = "PastPoint"
	case "CurrentNode":
		format.FormType = "CurrentNode"
	case "CurrentPoint":
		format.FormType = "CurrentPoint"
	case "ConditionNode":
		format.FormType = "ConditionNode"
	case "ConditionPoint":
		format.FormType = "ConditionPoint"
	case "Actuate":
		format.FormType = "Actuate"
	case "RegisterSensingData":
		format.FormType = "RegisterSensingData"
	case "ConnectNew":
		format.FormType = "ConnectNew"
	case "ConnectForModule":
		format.FormType = "ConnectForModule"
	case "AAA":
		format.FormType = "AAA"
	case "Disconn":
		format.FormType = "Disconn"
	}
	if err := encoder.Encode(format); err != nil {
		message.MyError(err, "syncFormatClient > "+command+" > encoder.Encode")
	}
}

func loadEnv() {
	// .envファイルの読み込み
	if err := godotenv.Load(os.Getenv("HOME") + "/.env"); err != nil {
		message.MyError(err, "loadEnv > godotenv.Load")
	}
}
