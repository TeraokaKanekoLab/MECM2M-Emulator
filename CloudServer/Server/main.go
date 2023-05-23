package main

import (
	"bytes"
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"mecm2m-Simulator/pkg/m2mapi"
	"mecm2m-Simulator/pkg/message"
	"mecm2m-Simulator/pkg/mserver"
	"mecm2m-Simulator/pkg/server"

	"github.com/joho/godotenv"

	_ "github.com/go-sql-driver/mysql"
)

const (
	protocol = "unix"

	// Home MEC Server かどうかの判定
	ServerID = "ServerID0001"
)

// 各種システムコンポーネントのソケットアドレスをグローバルに設定
var m2mApiSockAddr string

// var localMgrSockAddr string
var pnodeMgrSockAddr string
var aaaSockAddr string

// var localRepoSockAddr string
var graphDBSockAddr string
var sensingDBSockAddr string

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

	var socket_files server.ServerSocketFiles

	if err := json.Unmarshal(data, &socket_files); err != nil {
		message.MyError(err, "Failed to unmarshal json")
	}

	// MEC Server起動時に，MEC Server内のコンポーネント (API, LocalManager, PNManager, AAA, SensingDB, GraphDB, LocalRepo) のスレッドファイルを開けておく
	var socketFiles []string
	socketFiles = append(socketFiles,
		socket_files.M2MApi,
		socket_files.LocalMgr,
		socket_files.PNodeMgr,
		socket_files.AAA,
		socket_files.LocalRepo,
		socket_files.GraphDB,
		socket_files.SensingDB,
	)
	cleanup(socketFiles...)

	m2mApiSockAddr = socket_files.M2MApi
	//localMgrSockAddr = socket_files.LocalMgr
	pnodeMgrSockAddr = socket_files.PNodeMgr
	aaaSockAddr = socket_files.AAA
	//localRepoSockAddr = socket_files.LocalMgr
	graphDBSockAddr = socket_files.GraphDB
	sensingDBSockAddr = socket_files.SensingDB

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

		switch file {
		case m2mApiSockAddr:
			go m2mApi(conn)
		case pnodeMgrSockAddr:
			go pnodeMgr(conn)
		case aaaSockAddr:
			go aaa(conn)
		case graphDBSockAddr:
			go graphDB(conn)
		case sensingDBSockAddr:
			go sensingDB(conn)
		}
	}
}

func m2mApi(conn net.Conn) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	message.MyMessage("[MESSEGE] Call m2m api thread")

	for {
		// 型同期をして，型の種類に応じてスイッチ
		switch m := syncFormatServer(decoder, encoder); m.(type) {
		case *m2mapi.ResolvePoint:
			format := m.(*m2mapi.ResolvePoint)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "m2mApi > Point > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// Global GraphDBへリクエスト
			connDB, err := net.Dial(protocol, graphDBSockAddr)
			if err != nil {
				message.MyError(err, "m2mApi > Point > net.Dial")
			}
			decoderDB := gob.NewDecoder(connDB)
			encoderDB := gob.NewEncoder(connDB)

			syncFormatClient("Point", decoderDB, encoderDB)

			if err := encoderDB.Encode(format); err != nil {
				message.MyError(err, "m2mApi > Point > encoderDB.Encode")
			}
			message.MyWriteMessage(*format) //1. 同じ内容

			// GraphDB()によるDB検索

			// 受信する型は[]ResolvePoint
			ms := []m2mapi.ResolvePoint{}
			if err := decoderDB.Decode(&ms); err != nil {
				message.MyError(err, "m2mApi > Point > decoderDB.Decode")
			}
			message.MyReadMessage(ms)

			// 最終的な結果をM2M Appに送信する
			if err := encoder.Encode(&ms); err != nil {
				message.MyError(err, "m2mApi > Point > encoder.Encode")
				break
			}
			message.MyWriteMessage(ms)
		case *m2mapi.ResolveNode:
			format := m.(*m2mapi.ResolveNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "m2mApi > Node > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// Global GraphDBへリクエスト
			connDB, err := net.Dial(protocol, graphDBSockAddr)
			if err != nil {
				message.MyError(err, "m2mApi > Node > net.Dial")
			}
			decoderDB := gob.NewDecoder(connDB)
			encoderDB := gob.NewEncoder(connDB)

			syncFormatClient("Node", decoderDB, encoderDB)

			if err := encoderDB.Encode(format); err != nil {
				message.MyError(err, "m2mApi > Node > encoderDB.Encode")
			}
			message.MyWriteMessage((*format)) //1. 同じ内容

			// GraphDB()によるDB検索

			// 受信する型は[]ResolveNode
			ms := []m2mapi.ResolveNode{}
			if err := decoderDB.Decode(&ms); err != nil {
				message.MyError(err, "m2mApi > Node > decoderDB.Decode")
			}
			message.MyReadMessage(ms)

			// 最終的な結果をM2M Appに送信する
			if err := encoder.Encode(&ms); err != nil {
				message.MyError(err, "m2mApi > Node > encoder.Encode")
				break
			}
			message.MyWriteMessage(ms)
		case *m2mapi.ResolvePastNode:
			format := m.(*m2mapi.ResolvePastNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "m2mApi > PastNode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// どのVSNodeスレッドとやりとりするかを判別できるような仕組みが必要だが，現状はvsnode_1_1.sock
			connVS, err := net.Dial(protocol, "/tmp/mecm2m/vsnode_1_0001.sock")
			if err != nil {
				message.MyError(err, "m2mApi > PastNode > net.Dial")
			}
			decoderVS := gob.NewDecoder(connVS)
			encoderVS := gob.NewEncoder(connVS)

			syncFormatClient("PastNode", decoderVS, encoderVS)

			if err := encoderVS.Encode(format); err != nil {
				message.MyError(err, "m2mApi > PastNode > encoderVS.Encode")
			}
			message.MyWriteMessage(*format) //1. 同じ内容

			// VSNodeとのやりとり

			// 受信する型はResolvePastNode
			ms := m2mapi.ResolvePastNode{}
			if err := decoderVS.Decode(&ms); err != nil {
				message.MyError(err, "m2mApi > PastNode > decoderVS.Decode")
			}
			message.MyReadMessage(ms)

			// 最終的な結果をM2M Appに送信する
			if err := encoder.Encode(&ms); err != nil {
				message.MyError(err, "m2mApi > PastNode > encoder.Encode")
				break
			}
			message.MyWriteMessage(ms)
		case *m2mapi.ResolvePastPoint:
			format := m.(*m2mapi.ResolvePastPoint)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "m2mApi > PastPoint > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// どのVPointスレッドとやりとりするかを判別できるような仕組みが必要だが，現状はvpoint_1_1.sock
			connVP, err := net.Dial(protocol, "/tmp/mecm2m/vpoint_1_0001.sock")
			if err != nil {
				message.MyError(err, "m2mApi > PastPoint > net.Dial")
			}
			decoderVP := gob.NewDecoder(connVP)
			encoderVP := gob.NewEncoder(connVP)

			syncFormatClient("PastPoint", decoderVP, encoderVP)

			if err := encoderVP.Encode(format); err != nil {
				message.MyError(err, "m2mApi > PastPoint > encoderVP.Encode")
			}
			message.MyWriteMessage(*format) //1. 同じ内容

			// VPointとのやりとり

			// 受信する型はResolvePastPoint
			ms := m2mapi.ResolvePastPoint{}
			if err := decoderVP.Decode(&ms); err != nil {
				message.MyError(err, "m2mApi > PastPoint > decoderVP.Decode")
			}
			message.MyReadMessage(ms)

			// 最終的な結果をM2M Appに送信する
			if err := encoder.Encode(&ms); err != nil {
				message.MyError(err, "m2mApi > PastPoint > encoder.Encode")
				break
			}
			message.MyWriteMessage(ms)
		case *m2mapi.ResolveCurrentNode:
			format := m.(*m2mapi.ResolveCurrentNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "m2mApi > CurrentNode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// どのVSNodeスレッドとやりとりするかを判別できるような仕組みが必要だが，現状はvsnode_1_1.sock
			connVS, err := net.Dial(protocol, "/tmp/mecm2m/vsnode_1_0001.sock")
			if err != nil {
				message.MyError(err, "m2mApi > CurrentNode > net.Dial")
			}
			decoderVS := gob.NewDecoder(connVS)
			encoderVS := gob.NewEncoder(connVS)

			syncFormatClient("CurrentNode", decoderVS, encoderVS)

			if err := encoderVS.Encode(format); err != nil {
				message.MyError(err, "m2mApi > CurrentNode > encoderVS.Encode")
			}
			message.MyWriteMessage(*format) //1. 同じ内容

			// VSNodeとのやりとり

			// 受信する型はResolveCurrentNode
			ms := m2mapi.ResolveCurrentNode{}
			if err := decoderVS.Decode(&ms); err != nil {
				message.MyError(err, "m2mApi > CurrentNode > decoderVS.Decode")
			}
			message.MyReadMessage(ms)

			// 最終的な結果をM2M Appに送信する
			if err := encoder.Encode(&ms); err != nil {
				message.MyError(err, "m2mApi > CurrentNode > encoder.Encode")
				break
			}
			message.MyWriteMessage(ms)
		case *m2mapi.ResolveCurrentPoint:
			format := m.(*m2mapi.ResolveCurrentPoint)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "m2mApi > CurrentPoint > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// どのVPointスレッドとやりとりするかを判別できるような仕組みが必要だが，現状はvpoint_1_1.sock
			connVP, err := net.Dial(protocol, "/tmp/mecm2m/vpoint_1_0001.sock")
			if err != nil {
				message.MyError(err, "m2mApi > CurrentPoint > net.Dial")
			}
			decoderVP := gob.NewDecoder(connVP)
			encoderVP := gob.NewEncoder(connVP)

			syncFormatClient("CurrentPoint", decoderVP, encoderVP)

			if err := encoderVP.Encode(format); err != nil {
				message.MyError(err, "m2mApi > CurrentPoint > encoderVP.Encode")
			}
			message.MyWriteMessage(*format) //1. 同じ内容

			// VPointとのやりとり

			// 受信する型はResolveCurrentPoint
			ms := m2mapi.ResolveCurrentPoint{}
			if err := decoderVP.Decode(&ms); err != nil {
				message.MyError(err, "m2mApi > CurrentPoint > decoderVP.Decode")
			}
			message.MyReadMessage(ms)

			// 最終的な結果をM2M Appに送信する
			if err := encoder.Encode(&ms); err != nil {
				message.MyError(err, "m2mApi > CurrentPoint > encoder.Encode")
				break
			}
			message.MyWriteMessage(ms)
		case *m2mapi.ResolveConditionNode:
			format := m.(*m2mapi.ResolveConditionNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "m2mApi > ConditionNode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// どのVSNodeとやりとりするかを判別できるような仕組みが必要だが，現状はvsnode_1_1.sock
			connVS, err := net.Dial(protocol, "/tmp/mecm2m/vsnode_1_0001.sock")
			if err != nil {
				message.MyError(err, "m2mApi > ConditionNode > net.Dial")
			}
			decoderVS := gob.NewDecoder(connVS)
			encoderVS := gob.NewEncoder(connVS)

			syncFormatClient("ConditionNode", decoderVS, encoderVS)

			if err := encoderVS.Encode(format); err != nil {
				message.MyError(err, "m2mApi > ConditionNode > encoderVS.Encode")
			}
			message.MyWriteMessage(*format)
			fmt.Println("Wait for data notification...")

			// VSNodeからのデータ通知を受ける
			// 受信する型はDataForRegist
			ms := m2mapi.DataForRegist{}
			if err := decoderVS.Decode(&ms); err != nil {
				message.MyError(err, "m2mApi > ConditionNode > decoderVS.Decode")
			}
			message.MyReadMessage(ms)

			// 最終的な結果をM2M Appに送信する
			if err := encoder.Encode(&ms); err != nil {
				message.MyError(err, "m2mApi > ConditionNode > encoder.Encode")
				break
			}
			message.MyWriteMessage(ms)
		case string:
			if m == "exit" {
				// M2MAppでexitが入力されたら，breakする
				break
			}
		}
	}
}

// M2M Appと型同期をするための関数
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
	}
	return typeM
}

// 内部コンポーネント（DB，仮想モジュール）と型同期をするための関数
func syncFormatClient(command string, decoder *gob.Decoder, encoder *gob.Encoder) {
	m := &Format{}
	switch command {
	case "Point":
		m.FormType = "Point"
	case "Node":
		m.FormType = "Node"
	case "PastNode":
		m.FormType = "PastNode"
	case "PastPoint":
		m.FormType = "PastPoint"
	case "CurrentNode":
		m.FormType = "CurrentNode"
	case "CurrentPoint":
		m.FormType = "CurrentPoint"
	case "ConditionNode":
		m.FormType = "ConditionNode"
	case "RegisterSensingData":
		m.FormType = "RegisterSensingData"
	case "ConnectNew":
		m.FormType = "ConnectNew"
	case "ConnectForModule":
		m.FormType = "ConnectForModule"
	case "AAA":
		m.FormType = "AAA"
	case "Disconn":
		m.FormType = "Disconn"
	}
	if err := encoder.Encode(m); err != nil {
		message.MyError(err, "syncFormatClient > "+command+" > encoder.Encode")
	}
}

// PNode Manager Server
func pnodeMgr(conn net.Conn) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	message.MyMessage("[MESSAGE] Call PNode Manager thread")

	for {
		// 接続要求と切断要求の2種類でスイッチ
		switch m := syncFormatServer(decoder, encoder); m.(type) {
		case *mserver.ConnectNew:
			format := m.(*mserver.ConnectNew)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "pnodeMgr > ConnectNew > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// 1. 自分のAAAサーバで認証
			connAAA, err := net.Dial(protocol, aaaSockAddr)
			if err != nil {
				message.MyError(err, "pmnodeMgr > ConnectNewAAA > net.Dial")
			}
			decoderAAA := gob.NewDecoder(connAAA)
			encoderAAA := gob.NewEncoder(connAAA)

			syncFormatClient("AAA", decoderAAA, encoderAAA)

			mAAA := &server.AAA{
				VNodeID_n: format.VNodeID_n,
				HomeMECID: format.HomeMECID,
			}
			if err := encoderAAA.Encode(mAAA); err != nil {
				message.MyError(err, "pmnodeMgr > ConnectNew > encoderAAA.Encode")
			}
			message.MyWriteMessage(*mAAA)

			// AAAとのやりとり

			// 受信する型はAAA
			msAAA := server.AAA{}
			if err := decoderAAA.Decode(&msAAA); err != nil {
				message.MyError(err, "pmnodeMgr > ConnectNew > decoderAAA.Decode")
			}
			message.MyReadMessage(msAAA)

			if !msAAA.Status {
				message.MyMessage("AAA is disable")
				break
			}

			// 2. 接続要求を受けたPMNodeのHomeMECServerのPNManagerに接続要求
			// このリクエストはHomeMECServerのVMNodeHまで届く．これは，VMNodeHがPMNodeの位置情報を常に把握する必要があるから
			connPNMgrH, err := net.Dial(protocol, "/tmp/mecm2m/svr_1_pnodemgr.sock")
			if err != nil {
				message.MyError(err, "pmnodeMgr > ConnectNewPNMgrH > net.Dial")
			}
			decoderPNMgrH := gob.NewDecoder(connPNMgrH)
			encoderPNMgrH := gob.NewEncoder(connPNMgrH)

			syncFormatClient("ConnectForModule", decoderPNMgrH, encoderPNMgrH)

			mPNMgrH := &mserver.ConnectForModule{
				VNodeID_n:  format.VNodeID_n,
				PN_Type:    format.PN_Type,
				Time:       format.Time,
				Position:   format.Position,
				Capability: format.Capability,
				HomeMECID:  format.HomeMECID,
			}
			if err := encoderPNMgrH.Encode(mPNMgrH); err != nil {
				message.MyError(err, "pmnodeMgr > ConnectNew > encoderPNMgrH.Encode")
			}
			message.MyWriteMessage(*mPNMgrH)

			// Home MEC ServerのPNManagerとのやりとり

			// 受信する型はConnectForModule
			msPNMgrH := mserver.ConnectForModule{}
			if err := decoderPNMgrH.Decode(&msPNMgrH); err != nil {
				message.MyError(err, "pmnodeMgr > ConnectNew > decoderPNMgrH.Decode")
			}
			message.MyReadMessage(msPNMgrH)

			// 3. 旧MEC Serverに切断要求
			connDisconn, err := net.Dial(protocol, "/tmp/mecm2m/svr_2_pnodemgr.sock")
			if err != nil {
				message.MyError(err, "pmnodeMgr > ConnectNewDisconn > net.Dial")
			}
			decoderDisconn := gob.NewDecoder(connDisconn)
			encoderDisconn := gob.NewEncoder(connDisconn)

			syncFormatClient("Disconnect", decoderDisconn, encoderDisconn)

			mDisconnect := &mserver.Disconnect{
				VNodeID_n: format.VNodeID_n,
			}
			if err := encoderDisconn.Encode(mDisconnect); err != nil {
				message.MyError(err, "pmnodeMgr > ConnectNew > encoderDisconn.Encode")
			}
			message.MyWriteMessage(*mDisconnect)

			// 旧接続先のMEC Serverとのやりとり

			// 受信する型はDisconnect
			msDisconnect := mserver.Disconnect{}
			if err := decoderDisconn.Decode(&msDisconnect); err != nil {
				message.MyError(err, "pmnodeMgr > ConnectNew > decoderDisconnect.Decode")
			}
			message.MyReadMessage(msDisconnect)

			// 4. 新しい接続先であるPSinkに対応するVPointに接続要求
			// これも，VMNodeHと同様，VPointが接続先の情報を把握するため
			connNewVP, err := net.Dial(protocol, "/tmp/mecm2m/vpoint_3_0001.sock")
			if err != nil {
				message.MyError(err, "pmnodeMgr > ConnectNewNewVP > net.Dial")
			}
			decoderNewVP := gob.NewDecoder(connNewVP)
			encoderNewVP := gob.NewEncoder(connNewVP)

			syncFormatClient("ConnectForModule", decoderNewVP, encoderNewVP)

			mNewVP := &mserver.ConnectForModule{
				VNodeID_n:  format.VNodeID_n,
				PN_Type:    format.PN_Type,
				Time:       format.Time,
				Position:   format.Position,
				Capability: format.Capability,
				HomeMECID:  format.HomeMECID,
			}
			if err := encoderNewVP.Encode(mNewVP); err != nil {
				message.MyError(err, "pmnodeMgr > ConnectNew > encoderNewVP.Encode")
			}
			message.MyWriteMessage(*mNewVP)

			// 新しい接続先のVPointとのやりとり

			// 受信する型はConnectForModule
			msNewVP := mserver.ConnectForModule{}
			if err := decoderNewVP.Decode(&msNewVP); err != nil {
				message.MyError(err, "pmnodeMgr > ConnectNew > decoderNewVP.Decode")
			}
			message.MyReadMessage(msNewVP)

			// 5. VMNodeFに登録する

			// 最終的な接続応答をPMNodeへ返す
			ms := mserver.ConnectNew{}
			ms.Status = true
			ms.SessionKey = "Correct"
			if err := encoder.Encode(&ms); err != nil {
				message.MyError(err, "pmnodeMgr > ConnectNew > encoder.Encode")
			}
			message.MyWriteMessage(ms)
		case *mserver.ConnectForModule:
			format := m.(*mserver.ConnectForModule)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "pnodeMgr > ConnectForModule > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// VMNodeHスレッドに情報を渡す
			// どのVMNodeHスレッドとやりとりするかを判別できるような仕組みが必要だが，現状はvmnodeh_1_H_1.sock
			connVMH, err := net.Dial(protocol, "/tmp/mecm2m/vmnode_1_H_0001.sock")
			if err != nil {
				message.MyError(err, "pmnodeMgr > ConnectForModule > net.Dial")
			}
			decoderVMH := gob.NewDecoder(connVMH)
			encoderVMH := gob.NewEncoder(connVMH)

			syncFormatClient("ConnectForModule", decoderVMH, encoderVMH)

			m := &mserver.ConnectForModule{
				VNodeID_n:  format.VNodeID_n,
				PN_Type:    format.PN_Type,
				Time:       format.Time,
				Position:   format.Position,
				Capability: format.Capability,
				HomeMECID:  format.HomeMECID,
			}
			if err := encoderVMH.Encode(m); err != nil {
				message.MyError(err, "pmnodeMgr > ConnectForModule > encoderVMH.Encode")
			}
			message.MyWriteMessage(*m) //1. 同じ内容

			// VMNodeとのやりとり

			// 受信する型はConnectForModule
			ms := mserver.ConnectForModule{}
			if err := decoderVMH.Decode(&ms); err != nil {
				message.MyError(err, "pmnodeMgr > ConnectForModule > decoderVMH.Decode")
			}
			message.MyReadMessage(ms)

			// 最終的な結果を新しい接続先になるMEC ServerのPNManagerに送信する
			if err := encoder.Encode(&ms); err != nil {
				message.MyError(err, "pmnodeMgr > ConnectForModule > encoder.Encode")
				break
			}
			message.MyWriteMessage(ms)
		}
	}
}

// AAA Server
func aaa(conn net.Conn) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	message.MyMessage("[MESSAGE] Call AAA thread")

	for {
		m := syncFormatServer(decoder, encoder)
		format := m.(*server.AAA)
		if err := decoder.Decode(format); err != nil {
			if err == io.EOF {
				message.MyMessage("=== closed by client")
				break
			}
			message.MyError(err, "AAA > decoder.Decode")
			break
		}
		message.MyReadMessage(*format)

		// このサーバーがHomeMECServerであるかどうかでスイッチ
		if format.HomeMECID == "" || format.HomeMECID == ServerID {
			// HomeMECServerもしくは固定ノード
			ms := &server.AAA{
				Status: true,
			}
			if err := encoder.Encode(&ms); err != nil {
				message.MyError(err, "AAA > HomeMEC > encoder.Encode")
			}
			message.MyWriteMessage(ms)
		} else {
			// Foreing Server
			// Home MEC ServerのAAAに経由
			connHomeAAA, err := net.Dial(protocol, "/tmp/mecm2m/svr_1_aaa.sock")
			if err != nil {
				message.MyError(err, "AAA > ForeignMEC > net.Dial")
			}
			decoderAAA := gob.NewDecoder(connHomeAAA)
			encoderAAA := gob.NewEncoder(connHomeAAA)

			syncFormatClient("AAA", decoderAAA, encoderAAA)

			if err := encoderAAA.Encode(format); err != nil {
				message.MyError(err, "AAA > ForeignMEC > encoderAAA.Encode")
			}
			message.MyWriteMessage(*format)

			// Home MEC Server の AAA とのやりとり

			// 受信型はAAA
			ms := server.AAA{}
			if err := decoderAAA.Decode(&ms); err != nil {
				message.MyError(err, "AAA > ForeignMEC > decoderAAA.Decode")
			}
			message.MyReadMessage(ms)

			// 新しい接続先のAAAへ返信
			if err := encoder.Encode(&ms); err != nil {
				message.MyError(err, "AAA > ForeignMEC > encoder.Encode")
				break
			}
			message.MyWriteMessage(ms)
		}
	}
}

// GraphDB Server
func graphDB(conn net.Conn) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	message.MyMessage("[MESSEGE] Call GraphDB thread")

	for {
		// 型同期をして，型の種類に応じてスイッチ
		switch m := syncFormatServer(decoder, encoder); m.(type) {
		case *m2mapi.ResolvePoint:
			format := m.(*m2mapi.ResolvePoint)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "GraphDB > Point > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			var swlat, swlon, nelat, nelon float64
			swlat = format.SW.Lat
			swlon = format.SW.Lon
			nelat = format.NE.Lat
			nelon = format.NE.Lon

			// 指定された矩形範囲が少しでもカバー領域から外れていれば，クラウドサーバへリレー

			payload := `{"statements": [{"statement": "MATCH (ps:PSink)-[:isVirtualizedBy]->(vp:VPoint) WHERE ps.Position[0] > ` + strconv.FormatFloat(swlat, 'f', 4, 64) + ` and ps.Position[1] > ` + strconv.FormatFloat(swlon, 'f', 4, 64) + ` and ps.Position[0] <= ` + strconv.FormatFloat(nelat, 'f', 4, 64) + ` and ps.Position[1] <= ` + strconv.FormatFloat(nelon, 'f', 4, 64) + ` return vp.VPointID;"}]}`
			// 今後はクラウドサーバ用の分岐が必要
			var url string
			url = "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_GLOBAL_PASSWORD") + "@" + "localhost:" + os.Getenv("NEO4J_GLOBAL_PORT_GOLANG") + "/db/data/transaction/commit"
			datas := listenServer(payload, url)

			pss := []m2mapi.ResolvePoint{}
			for _, data := range datas {
				dataArray := data.([]interface{})
				fmt.Println(dataArray)
				ps := m2mapi.ResolvePoint{}
				ps.VPointID_n = dataArray[0].(string)
				flag := 0
				for _, p := range pss {
					if p.VPointID_n == ps.VPointID_n {
						flag = 1
					}
				}
				if flag == 0 {
					pss = append(pss, ps)
				}
			}

			if err := encoder.Encode(&pss); err != nil {
				message.MyError(err, "GraphDB > Point > encoder.Encode")
			}
			message.MyWriteMessage(pss)
		case *m2mapi.ResolveNode:
			format := m.(*m2mapi.ResolveNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "GraphDB > Node > decoder.Decode")
				break
			}
			message.MyReadMessage(&format)

			var vpointid_n string
			vpointid_n = "\\\"" + format.VPointID_n + "\\\""
			caps := format.CapsInput
			var format_caps []string
			for _, cap := range caps {
				cap = "\\\"" + cap + "\\\""
				format_caps = append(format_caps, cap)
			}
			payload := `{"statements": [{"statement": "MATCH (vp:VPoint {VPointID: ` + vpointid_n + `})-[:aggregates]->(vn:VNode)-[:isPhysicalizedBy]->(pn:PNode) WHERE pn.Capability IN [` + strings.Join(format_caps, ", ") + `] return vn.VNodeID, pn.Capability;"}]}`

			var url string
			url = "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_GLOBAL_PASSWORD") + "@" + "localhost:" + os.Getenv("NEO4J_GLOBAL_PORT_GOLANG") + "/db/data/transaction/commit"
			datas := listenServer(payload, url)

			nds := []m2mapi.ResolveNode{}
			for _, data := range datas {
				dataArray := data.([]interface{})
				fmt.Println(dataArray)
				pn := m2mapi.ResolveNode{}
				capability := dataArray[0].(string)
				// CapOutputを1つにするか配列にして複数まとめられるようにするか要検討
				//pn.CapOutput = append(pn.CapOutput, capability)
				pn.CapOutput = capability
				pn.VNodeID_n = dataArray[1].(string)
				flag := 0
				for _, p := range nds {
					if p.VNodeID_n == pn.VNodeID_n {
						flag = 1
					} /*else {
						// CapOutputを1つにするか配列にして複数まとめられるようにするか要検討
						p.Capabilities = append(p.Capabilities, capability)
					}*/
				}
				if flag == 0 {
					nds = append(nds, pn)
				}
			}

			if err := encoder.Encode(&nds); err != nil {
				message.MyError(err, "GraphDB > Node > encoder.Encode")
			}
			message.MyWriteMessage(nds)
		}
	}
}

// SensingDB Server
func sensingDB(conn net.Conn) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	message.MyMessage("[MESSEGE] Call SensingDB thread")

	for {
		// 型同期をして，型の種類に応じてスイッチ
		switch m := syncFormatServer(decoder, encoder); m.(type) {
		case *m2mapi.ResolvePastNode:
			format := m.(*m2mapi.ResolvePastNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "SensingDB > PastNode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			var vnodeid_n, cap, start, end string
			vnodeid_n = format.VNodeID_n
			cap = format.Capability
			start = format.Period.Start
			end = format.Period.End

			// SensingDBを開く
			// "root:password@tcp(127.0.0.1:3306)/testdb"
			mysql_path := os.Getenv("MYSQL_USERNAME") + ":" + os.Getenv("MYSQL_PASSWORD") + "@tcp(127.0.0.1:" + os.Getenv("MYSQL_PORT") + ")/" + os.Getenv("MYSQL_DB")
			DBConnection, err := sql.Open("mysql", mysql_path)
			if err != nil {
				message.MyError(err, "SensingDB > PastNode > sql.Open")
			}
			defer DBConnection.Close()
			if err := DBConnection.Ping(); err != nil {
				message.MyError(err, "SensingDB > PastNode > DBConnection.Ping")
			} else {
				message.MyMessage("DB Connection Success")
			}

			var cmd string
			cmd = "SELECT * FROM " + os.Getenv("MYSQL_TABLE") + " WHERE PNodeID = \"" + vnodeid_n + "\" AND Capability = \"" + cap + "\" AND Timestamp > \"" + start + "\" AND Timestamp < \"" + end + "\";"

			rows, err := DBConnection.Query(cmd)
			if err != nil {
				message.MyError(err, "SensingDB > PastNode > DBConnection.Query")
			}
			defer rows.Close()

			sd := m2mapi.ResolvePastNode{}
			for rows.Next() {
				field := []string{"0", "0", "0", "0", "0", "0", "0", "0", "0", "0"}
				// PNodeID, Capability, Timestamp, Value, PSinkID, ServerID, Lat, Lon, VNodeID, VPointID
				err := rows.Scan(&field[0], &field[1], &field[2], &field[3], &field[4], &field[5], &field[6], &field[7], &field[8], &field[9])
				if err != nil {
					message.MyError(err, "SensingDB > PastNode > rows.Scan")
				}
				sd.VNodeID_n = field[0]
				valFloat, _ := strconv.ParseFloat(field[3], 64)
				val := m2mapi.Value{Capability: field[1], Time: field[2], Value: valFloat}
				sd.Values = append(sd.Values, val)
			}

			if err := encoder.Encode(&sd); err != nil {
				message.MyError(err, "SensingDB > PastNode > encoder.Encode")
			}
			message.MyWriteMessage(sd)
		case *m2mapi.ResolvePastPoint:
			format := m.(*m2mapi.ResolvePastPoint)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "SensingDB > PastPoint > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			var vpointid_n, cap, start, end string
			vpointid_n = format.VPointID_n
			cap = format.Capability
			start = format.Period.Start
			end = format.Period.End

			// SensingDBを開く
			mysql_path := os.Getenv("MYSQL_USERNAME") + ":" + os.Getenv("MYSQL_PASSWORD") + "@tcp(127.0.0.1:" + os.Getenv("MYSQL_PORT") + ")/" + os.Getenv("MYSQL_DB")
			DBConnection, err := sql.Open("mysql", mysql_path)
			if err != nil {
				message.MyError(err, "SensingDB > PastNode > sql.Open")
			}
			defer DBConnection.Close()
			if err := DBConnection.Ping(); err != nil {
				message.MyError(err, "SensingDB > PastNode > DBConnection.Ping")
			} else {
				message.MyMessage("DB Connection Success")
			}

			var cmd string
			cmd = "SELECT * FROM " + os.Getenv("MYSQL_TABLE") + " WHERE PSinkID = \"" + vpointid_n + "\" AND Capability = \"" + cap + "\" AND Timestamp > \"" + start + "\" AND Timestamp < \"" + end + "\";"

			rows, err := DBConnection.Query(cmd)
			if err != nil {
				message.MyError(err, "SensingDB > PastNode > DBConnection.Query")
			}
			defer rows.Close()

			sd := m2mapi.ResolvePastPoint{}
			for rows.Next() {
				field := []string{"0", "0", "0", "0", "0", "0", "0", "0", "0", "0"}
				// PNodeID, Capability, Timestamp, Value, PSinkID, ServerID, Lat, Lon, VNodeID, VPointID
				err := rows.Scan(&field[0], &field[1], &field[2], &field[3], &field[4], &field[5], &field[6], &field[7], &field[8], &field[9])
				if err != nil {
					message.MyError(err, "SensingDB > PastNode > rows.Scan")
				}
				if len(sd.Datas) < 1 {
					sensordata := m2mapi.SensorData{
						VNodeID_n: field[0],
					}
					valFloat, _ := strconv.ParseFloat(field[3], 64)
					val := m2mapi.Value{
						Capability: field[1], Time: field[2], Value: valFloat,
					}
					sensordata.Values = append(sensordata.Values, val)
					sd.Datas = append(sd.Datas, sensordata)
				} else {
					flag := 0
					for i, data := range sd.Datas {
						if field[0] == data.VNodeID_n {
							valFloat, _ := strconv.ParseFloat(field[3], 64)
							val := m2mapi.Value{
								Capability: field[1], Time: field[2], Value: valFloat,
							}
							data.Values = append(data.Values, val)
							sd.Datas[i] = data
							flag = 1
						}
					}
					if flag == 0 {
						sensordata := m2mapi.SensorData{
							VNodeID_n: field[0],
						}
						valFloat, _ := strconv.ParseFloat(field[3], 64)
						val := m2mapi.Value{
							Capability: field[1], Time: field[2], Value: valFloat,
						}
						sensordata.Values = append(sensordata.Values, val)
						sd.Datas = append(sd.Datas, sensordata)
					}
				}
			}

			if err := encoder.Encode(&sd); err != nil {
				message.MyError(err, "SensingDB > PastPoint > encoder.Encode")
			}
			message.MyWriteMessage(sd)
		case *m2mapi.DataForRegist:
			format := m.(*m2mapi.DataForRegist)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "SensingDB > RegisterSensingData > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			var PNodeID, Capability, Timestamp, Value, PSinkID, ServerID, Lat, Lon, VNodeID, VPointID string
			PNodeID = format.PNodeID
			Capability = format.Capability
			Timestamp = format.Timestamp
			Value = format.Value
			PSinkID = format.PSinkID
			ServerID = format.ServerID
			Lat = format.Lat
			Lon = format.Lon
			VNodeID = format.VNodeID
			VPointID = format.VPointID

			// SensingDBを開く
			mysql_path := os.Getenv("MYSQL_USERNAME") + ":" + os.Getenv("MYSQL_PASSWORD") + "@tcp(127.0.0.1:" + os.Getenv("MYSQL_PORT") + ")/" + os.Getenv("MYSQL_DB")
			DBConnection, err := sql.Open("mysql", mysql_path)
			if err != nil {
				message.MyError(err, "SensingDB > RegisterSensingData > sql.Open")
			}
			defer DBConnection.Close()
			if err := DBConnection.Ping(); err != nil {
				message.MyError(err, "SensingDB > RegisterSensingData > DBConnection.Ping")
			} else {
				message.MyMessage("DB Connection Success")
			}

			// PNodeID, Capability, Timestamp, Value, PSinkID, ServerID, Lat, Lon, VNodeID, VPointID
			var cmd string
			cmd = "INSERT INTO " + os.Getenv("MYSQL_TABLE") + "(PNodeID,Capability,Timestamp,Value,PSinkID,ServerID,Lat,Lon,VNodeID,VPointID) VALUES(?,?,?,?,?,?,?,?,?,?);"

			in, err := DBConnection.Prepare(cmd)
			if err != nil {
				message.MyError(err, "SensingDB > RegisterSensingData > DBConnection.Prepare")
			}
			if _, errExec := in.Exec(PNodeID, Capability, Timestamp, Value, PSinkID, ServerID, Lat, Lon, VNodeID, VPointID); errExec == nil {
				message.MyMessage("Complete Data Registration!")
			} else {
				fmt.Println("Faild to register data", errExec)
			}
			in.Close()
		}
	}
}

// MEC/Cloud Server へGraph DBの解決要求
func listenServer(payload string, url string) []interface{} {
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer([]byte(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")

	client := new(http.Client)
	resp, err := client.Do(req)
	if err != nil {
		message.MyError(err, "ListenServer > client.Do")
	}
	defer resp.Body.Close()
	byteArray, _ := ioutil.ReadAll(resp.Body)

	var datas []interface{}
	if strings.Contains(url, "neo4j") {
		datas = bodyNeo4j(byteArray)
	} else {
		datas = bodyGraphQL(byteArray)
	}
	return datas
}

// Query Server から返ってきた　Reponse を探索し,中身を返す
func bodyNeo4j(byteArray []byte) []interface{} {
	var jsonBody map[string]interface{}
	if err := json.Unmarshal(byteArray, &jsonBody); err != nil {
		message.MyError(err, "BodyNeo4j > json.Unmarshal")
		return nil
	}
	var datas []interface{}
	// message.MyMessage("jsonBody: ", jsonBody)
	for _, v1 := range jsonBody {
		for _, v2 := range v1.([]interface{}) {
			for k3, v3 := range v2.(map[string]interface{}) {
				if k3 == "data" {
					for _, v4 := range v3.([]interface{}) {
						for k5, v5 := range v4.(map[string]interface{}) {
							if k5 == "row" {
								datas = append(datas, v5)
							}
						}
					}
				}
			}
		}
	}
	return datas
}

func bodyGraphQL(byteArray []byte) []interface{} {
	var jsonBody map[string]interface{}
	if err := json.Unmarshal(byteArray, &jsonBody); err != nil {
		message.MyError(err, "BodyGraphQL > json.Unmarshal")
		return nil
	}
	var values []interface{}
	//fmt.Println(jsonBody)
	for _, v1 := range jsonBody {
		for _, v2 := range v1.(map[string]interface{}) {
			switch x := v2.(type) {
			case []interface{}:
				values = v2.([]interface{})
			case map[string]interface{}:
				for _, v3 := range v2.(map[string]interface{}) {
					values = append(values, v3)
				}
			default:
				fmt.Println("Format Assertion False: ", x)
			}
		}
	}
	return values
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
