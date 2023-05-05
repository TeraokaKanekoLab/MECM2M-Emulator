package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mecm2m-Simulator/pkg/m2mapi"
	"mecm2m-Simulator/pkg/message"
	"mecm2m-Simulator/pkg/mserver"

	"github.com/joho/godotenv"

	_ "github.com/go-sql-driver/mysql"
)

const (
	protocol = "unix"

	m2mApiSockAddr    = "/tmp/mecm2m/pmnode_1_m2mapi.sock"
	localMgrSockAddr  = "/tmp/mecm2m/pmnode_1_localmgr.sock"
	pnodeMgrSockAddr  = "/tmp/mecm2m/svr_1_pnodemgr.sock" //PMNodeであり，接続要求を送信するので，接続先のMEC ServerのPNManagerのスレッドファイルを選択する
	aaaSockAddr       = "/tmp/mecm2m/pmnode_1_aaa.sock"
	localRepoSockAddr = "/tmp/mecm2m/pmnode_1_localrepo.sock"
	graphDBSockAddr   = "/tmp/mecm2m/pmnode_1_graphdb.sock"
	sensingDBSockAddr = "/tmp/mecm2m/pmnode_1_sensingdb.sock"

	//vsnodePort = "8080"
)

type Format struct {
	FormType string
}

type Position struct {
	Lat float64
	Lon float64
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
	//PMNode起動時に，PMNode内のコンポーネント (API, LocalManager, PNManager, AAA, SensingDB, GraphDB, LocalRepo) のスレッドファイルを開けておく
	//configを読み込むことで，1秒ごとの緯度経度が格納された配列が用意される．
	var socketFiles []string
	socketFiles = append(socketFiles,
		//m2mApiSockAddr,
		localMgrSockAddr,
		pnodeMgrSockAddr,
		//aaaSockAddr,
		//localRepoSockAddr,
		//graphDBSockAddr,
		//sensingDBSockAddr,
	)
	//cleanup(socketFiles...)

	var pmnodePosition []Position
	pmnodePosition = append(pmnodePosition,
		Position{35.5300, 139.5300},
		Position{35.5310, 139.5310},
		Position{35.5410, 139.5410},
		Position{35.5420, 139.5420},
	)

	//接続切り替え地点
	switchPosition := Position{35.54, 139.54}

	//移動の表現
	mainContext := context.Background()
	cancelContext, cancelFunc := context.WithCancel(mainContext)

	go movement(pmnodePosition, switchPosition, cancelFunc)

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGALRM)
	go func() {
		<-quit
		fmt.Println("ctrl-c pressed!")
		close(quit)
		cleanup(socketFiles...)
		os.Exit(0)
	}()

	for _, file := range socketFiles {
		if file == pnodeMgrSockAddr {
			go pnodeMgr(file, cancelContext)
		} else {
			go initialize(file)
		}
	}
	fmt.Scanln()
}

func initialize(file string) {
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
		case graphDBSockAddr:
			go graphDB(conn)
		case sensingDBSockAddr:
			go sensingDB(conn)
		}
	}
}

func movement(pmnodePosition []Position, switchPosition Position, cancelFunc context.CancelFunc) {
	//時間間隔指定
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()

	//シグナル受信用チャネル
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer signal.Stop(sig)

	for _, position := range pmnodePosition {
		select {
		case <-t.C:
			if position.Lat > switchPosition.Lat && position.Lon > switchPosition.Lon {
				//PMNodeのPNodeManagerからMEC ServerのPNodeManagerへ接続切り替え要求を投げる
				fmt.Println(position)
				cancelFunc()
			}
		//シグナルを受信した場合
		case s := <-sig:
			switch s {
			case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
				fmt.Println("Stop!")
				return
			}
		}
	}
}

func m2mApi(conn net.Conn) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	message.MyMessage("[MESSEGE] Call m2m api thread")

	for {
		//型同期をして，型の種類に応じてスイッチ
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

			//GraphDB()によるDB検索

			//受信する型は[]ResolvePoint
			ms := []m2mapi.ResolvePoint{}
			if err := decoderDB.Decode(&ms); err != nil {
				message.MyError(err, "m2mApi > Point > decoderDB.Decode")
			}
			message.MyReadMessage(ms)

			//最終的な結果をM2M Appに送信する
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

			//GraphDB()によるDB検索

			//受信する型は[]ResolveNode
			ms := []m2mapi.ResolveNode{}
			if err := decoderDB.Decode(&ms); err != nil {
				message.MyError(err, "m2mApi > Node > decoderDB.Decode")
			}
			message.MyReadMessage(ms)

			//最終的な結果をM2M Appに送信する
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

			//どのVSNodeスレッドとやりとりするかを判別できるような仕組みが必要だが，現状はvsnode_1_1.sock
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

			//VSNodeとのやりとり

			// 受信する型はResolvePastNode
			ms := m2mapi.ResolvePastNode{}
			if err := decoderVS.Decode(&ms); err != nil {
				message.MyError(err, "m2mApi > PastNode > decoderVS.Decode")
			}
			message.MyReadMessage(ms)

			//最終的な結果をM2M Appに送信する
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

			//どのVPointスレッドとやりとりするかを判別できるような仕組みが必要だが，現状はvpoint_1_1.sock
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

			//VPointとのやりとり

			// 受信する型はResolvePastPoint
			ms := m2mapi.ResolvePastPoint{}
			if err := decoderVP.Decode(&ms); err != nil {
				message.MyError(err, "m2mApi > PastPoint > decoderVP.Decode")
			}
			message.MyReadMessage(ms)

			//最終的な結果をM2M Appに送信する
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

			//どのVSNodeスレッドとやりとりするかを判別できるような仕組みが必要だが，現状はvsnode_1_1.sock
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

			//VSNodeとのやりとり

			// 受信する型はResolveCurrentNode
			ms := m2mapi.ResolveCurrentNode{}
			if err := decoderVS.Decode(&ms); err != nil {
				message.MyError(err, "m2mApi > CurrentNode > decoderVS.Decode")
			}
			message.MyReadMessage(ms)

			//最終的な結果をM2M Appに送信する
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

			//どのVPointスレッドとやりとりするかを判別できるような仕組みが必要だが，現状はvpoint_1_1.sock
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

			//VPointとのやりとり

			// 受信する型はResolveCurrentPoint
			ms := m2mapi.ResolveCurrentPoint{}
			if err := decoderVP.Decode(&ms); err != nil {
				message.MyError(err, "m2mApi > CurrentPoint > decoderVP.Decode")
			}
			message.MyReadMessage(ms)

			//最終的な結果をM2M Appに送信する
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

			//どのVSNodeとやりとりするかを判別できるような仕組みが必要だが，現状はvsnode_1_1.sock
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

			//VSNodeからのデータ通知を受ける
			//受信する型はDataForRegist
			ms := m2mapi.DataForRegist{}
			if err := decoderVS.Decode(&ms); err != nil {
				message.MyError(err, "m2mApi > ConditionNode > decoderVS.Decode")
			}
			message.MyReadMessage(ms)

			//最終的な結果をM2M Appに送信する
			if err := encoder.Encode(&ms); err != nil {
				message.MyError(err, "m2mApi > ConditionNode > encoder.Encode")
				break
			}
			message.MyWriteMessage(ms)
		case string:
			if m == "exit" {
				//M2MAppでexitが入力されたら，breakする
				break
			}
		}
	}
}

//M2M Appと型同期をするための関数
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
	}
	return typeM
}

//内部コンポーネント（DB，仮想モジュール）と型同期をするための関数
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
	}
	if err := encoder.Encode(m); err != nil {
		message.MyError(err, "syncFormatClient > "+command+" > encoder.Encode")
	}
}

//PNode Manager Server
func pnodeMgr(file string, cancelContext context.Context) {
	message.MyMessage("[MESSAGE] Call PNode Manager thread")

	conn, err := net.Dial(protocol, file)
	if err != nil {
		message.MyError(err, "pnodeMgr > net.Dial")
	}
	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	<-cancelContext.Done()
	//接続先MEC Serverへ接続切り替え要求を投げる
	//ここでは接続先MEC Serverは固定値とする

	//初めに接続先MEC Serverと型同期をする
	syncFormatClient("ConnectNew", decoder, encoder)

	m := &mserver.ConnectNew{
		VNodeID_n:  "PMNodeID0001",
		PN_Type:    "Car",
		Time:       "2023-04-05 12:00:00",
		Position:   mserver.SquarePoint{Lat: 35.53, Lon: 139.54},
		Capability: "Car",
		HomeMECID:  "Server1",
	}
	if err := encoder.Encode(m); err != nil {
		message.MyError(err, "pnodeMgr > encoder.Encode")
	}
	message.MyWriteMessage(m)

	//接続応答を受信
	ms := mserver.ConnectNew{}
	if err := decoder.Decode(&ms); err != nil {
		message.MyError(err, "pnodeMgr > decoder.Decode")
	}
	message.MyReadMessage(ms)
}

//GraphDB Server
func graphDB(conn net.Conn) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	message.MyMessage("[MESSEGE] Call GraphDB thread")

	for {
		//型同期をして，型の種類に応じてスイッチ
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

			payload := `{"statements": [{"statement": "MATCH (ps:PSink)-[:isVirtualizedWith]->(vp:VPoint) WHERE ps.Lat > ` + strconv.FormatFloat(swlat, 'f', 4, 64) + ` and ps.Lon > ` + strconv.FormatFloat(swlon, 'f', 4, 64) + ` and ps.Lat < ` + strconv.FormatFloat(nelat, 'f', 4, 64) + ` and ps.Lon < ` + strconv.FormatFloat(nelon, 'f', 4, 64) + ` return ps.PSinkID, vp.Address;"}]}`
			//今後はクラウドサーバ用の分岐が必要
			var url string
			url = "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_PASSWORD") + "@" + "localhost:" + os.Getenv("NEO4J_PORT") + "/db/data/transaction/commit"
			datas := listenServer(payload, url)

			pss := []m2mapi.ResolvePoint{}
			for _, data := range datas {
				dataArray := data.([]interface{})
				ps := m2mapi.ResolvePoint{}
				ps.VPointID_n = dataArray[0].(string)
				ps.Address = dataArray[1].(string)
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
			payload := `{"statements": [{"statement": "MATCH (ps:PSink {PSinkID: ` + vpointid_n + `})-[:requestsViaDevApi]->(pn:PNode) WHERE pn.Capability IN [` + strings.Join(format_caps, ", ") + `] return pn.Capability, pn.PNodeID;"}]}`
			//今後はクラウドサーバ用の分岐が必要
			var url string
			url = "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_PASSWORD") + "@" + "localhost:" + os.Getenv("NEO4J_PORT") + "/db/data/transaction/commit"
			datas := listenServer(payload, url)

			nds := []m2mapi.ResolveNode{}
			for _, data := range datas {
				dataArray := data.([]interface{})
				pn := m2mapi.ResolveNode{}
				capability := dataArray[0].(string)
				//CapOutputを1つにするか配列にして複数まとめられるようにするか要検討
				//pn.CapOutput = append(pn.CapOutput, capability)
				pn.CapOutput = capability
				pn.VNodeID_n = dataArray[1].(string)
				flag := 0
				for _, p := range nds {
					if p.VNodeID_n == pn.VNodeID_n {
						flag = 1
					} /*else {
						//CapOutputを1つにするか配列にして複数まとめられるようにするか要検討
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

//SensingDB Server
func sensingDB(conn net.Conn) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	message.MyMessage("[MESSEGE] Call SensingDB thread")

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
				message.MyError(err, "SensingDB > PastNode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			var vnodeid_n, cap, start, end string
			vnodeid_n = format.VNodeID_n
			cap = format.Capability
			start = format.Period.Start
			end = format.Period.End

			//SensingDBを開く
			DBConnection, err := sql.Open("mysql", "root:password@tcp(127.0.0.1:3306)/testdb")
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
			cmd = "SELECT * FROM sensordata WHERE PNodeID = \"" + vnodeid_n + "\" AND Capability = \"" + cap + "\" AND Timestamp > \"" + start + "\" AND Timestamp < \"" + end + "\";"

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

			//SensingDBを開く
			DBConnection, err := sql.Open("mysql", "root:password@tcp(127.0.0.1:3306)/testdb")
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
			cmd = "SELECT * FROM sensordata WHERE PSinkID = \"" + vpointid_n + "\" AND Capability = \"" + cap + "\" AND Timestamp > \"" + start + "\" AND Timestamp < \"" + end + "\";"

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

			//SensingDBを開く
			DBConnection, err := sql.Open("mysql", "root:password@tcp(127.0.0.1:3306)/testdb")
			if err != nil {
				message.MyError(err, "SensingDB > RegisterSensingData > sql.Open")
			}
			defer DBConnection.Close()
			if err := DBConnection.Ping(); err != nil {
				message.MyError(err, "SensingDB > RegisterSensingData > DBConnection.Ping")
			} else {
				message.MyMessage("DB Connection Success")
			}

			//PNodeID, Capability, Timestamp, Value, PSinkID, ServerID, Lat, Lon, VNodeID, VPointID
			var cmd string
			cmd = "INSERT INTO sensordata(PNodeID,Capability,Timestamp,Value,PSinkID,ServerID,Lat,Lon,VNodeID,VPointID) VALUES(?,?,?,?,?,?,?,?,?,?);"

			in, err := DBConnection.Prepare(cmd)
			if err != nil {
				message.MyError(err, "SensingDB > RegisterSensingData > DBConnection.Prepare")
			}
			if _, errExec := in.Exec(PNodeID, Capability, Timestamp, Value, PSinkID, ServerID, Lat, Lon, VNodeID, VPointID); errExec == nil {
				message.MyMessage("Complete Data Registration!")
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
	//message.MyMessage("jsonBody: ", jsonBody)
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
	//.envファイルの読み込み
	if err := godotenv.Load(os.Getenv("HOME") + "/.env"); err != nil {
		log.Fatal(err)
	}
	mes := os.Getenv("SAMPLE_MESSAGE")
	//fmt.Printf("\x1b[32m%v\x1b[0m\n", message)
	message.MyMessage(mes)
}
