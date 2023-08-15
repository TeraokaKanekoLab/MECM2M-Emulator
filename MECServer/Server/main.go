package main

import (
	"bytes"
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"mecm2m-Emulator/pkg/m2mapi"
	"mecm2m-Emulator/pkg/message"
	"mecm2m-Emulator/pkg/mserver"
	"mecm2m-Emulator/pkg/server"
	"mecm2m-Emulator/pkg/vpoint"

	"github.com/joho/godotenv"

	_ "github.com/go-sql-driver/mysql"
)

const (
	protocol                 = "unix"
	globalGraphDBSockAddr    = "/tmp/mecm2m/svr_0_m2mapi.sock"
	socket_address_root      = "/tmp/mecm2m/"
	link_socket_address_root = "/tmp/mecm2m/link-process/"

	// Home MEC Server かどうかの判定
	ServerID = "ServerID0001"
)

// 自身のカバー領域情報をキャッシュ
var covered_area = make(map[string][]float64)
var mu sync.Mutex

// 各種システムコンポーネントのソケットアドレスをグローバルに設定
// ----------------------------------------------------
var m2mApiSockAddr string

// var localMgrSockAddr string
var pnodeMgrSockAddr string
var aaaSockAddr string

// var localRepoSockAddr string
var graphDBSockAddr string
var sensingDBSockAddr string

// ----------------------------------------------------

// このプロセスが動いているMEC Serverの番号をグローバルに設定
var server_num string

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
	// コマンドライン引数にソケットファイル群をまとめたファイルを指定して，初めにそのファイルを読み込む
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

	// サーバ番号の読み出し
	server_num_first_index := strings.LastIndex(socket_file_name, "_")
	server_num_last_index := strings.LastIndex(socket_file_name, ".")
	server_num = socket_file_name[server_num_first_index+1 : server_num_last_index]

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
		switch m2mApiCommand := syncFormatServer(decoder, encoder); m2mApiCommand.(type) {
		case *m2mapi.ResolvePoint:
			format := m2mApiCommand.(*m2mapi.ResolvePoint)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "m2mApi > Point > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// キャッシュ情報をもとに，ローカルかグローバルか判断する．キャッシュがない場合は，ローカルで検索する．
			// キャッシュの検索はここで行う
			if _, ok := covered_area["MINLAT"]; !ok {
				local_url := "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_LOCAL_PASSWORD") + "@" + "localhost:" + os.Getenv("NEO4J_LOCAL_PORT_GOLANG") + "/db/data/transaction/commit"

				mu.Lock()
				// エッジサーバのカバー領域を検索する
				// MINLAT, MAXLAT, MINLON, MAXLONのそれぞれを持つレコードを検索
				min_lat_payload := `{"statements": [{"statement": "MATCH (a: Area) WHERE size(a.SW) > 1 WITH min(a.SW[0]) as minLat MATCH (a:Area) WHERE a.SW[0] = minLat RETURN a ORDER BY a.SW[1] ASC LIMIT 1;"}]}`
				min_lat_datas := listenServer(min_lat_payload, local_url)
				for _, data := range min_lat_datas {
					dataArray := data.([]interface{})
					dataArrayInterface := dataArray[0]
					min_lat_record := dataArrayInterface.(map[string]interface{})
					min_lat_interface := min_lat_record["SW"].([]interface{})
					var min_lat []float64
					for _, v := range min_lat_interface {
						min_lat = append(min_lat, v.(float64))
					}
					covered_area["MINLAT"] = min_lat
				}

				min_lon_payload := `{"statements": [{"statement": "MATCH (a: Area) WHERE size(a.SW) > 1 WITH min(a.SW[1]) as minLon MATCH (a:Area) WHERE a.SW[1] = minLon RETURN a ORDER BY a.SW[0] ASC LIMIT 1;"}]}`
				min_lon_datas := listenServer(min_lon_payload, local_url)
				for _, data := range min_lon_datas {
					dataArray := data.([]interface{})
					dataArrayInterface := dataArray[0]
					min_lon_record := dataArrayInterface.(map[string]interface{})
					min_lon_interface := min_lon_record["SW"].([]interface{})
					var min_lon []float64
					for _, v := range min_lon_interface {
						min_lon = append(min_lon, v.(float64))
					}
					covered_area["MINLON"] = min_lon
				}

				max_lat_payload := `{"statements": [{"statement": "MATCH (a: Area) WHERE size(a.NE) > 1 WITH max(a.NE[0]) as maxLat MATCH (a:Area) WHERE a.NE[0] = maxLat RETURN a ORDER BY a.NE[1] DESC LIMIT 1;"}]}`
				max_lat_datas := listenServer(max_lat_payload, local_url)
				for _, data := range max_lat_datas {
					dataArray := data.([]interface{})
					dataArrayInterface := dataArray[0]
					max_lat_record := dataArrayInterface.(map[string]interface{})
					max_lat_interface := max_lat_record["NE"].([]interface{})
					var max_lat []float64
					for _, v := range max_lat_interface {
						max_lat = append(max_lat, v.(float64))
					}
					covered_area["MAXLAT"] = max_lat
				}

				max_lon_payload := `{"statements": [{"statement": "MATCH (a: Area) WHERE size(a.NE) > 1 WITH max(a.NE[1]) as maxLon MATCH (a:Area) WHERE a.NE[1] = maxLon RETURN a ORDER BY a.NE[0] DESC LIMIT 1;"}]}`
				max_lon_datas := listenServer(max_lon_payload, local_url)
				for _, data := range max_lon_datas {
					dataArray := data.([]interface{})
					dataArrayInterface := dataArray[0]
					max_lon_record := dataArrayInterface.(map[string]interface{})
					max_lon_interface := max_lon_record["NE"].([]interface{})
					var max_lon []float64
					for _, v := range max_lon_interface {
						max_lon = append(max_lon, v.(float64))
					}
					covered_area["MAXLON"] = max_lon
				}

				mu.Unlock()
				fmt.Println("Cache covered area: ", covered_area)
				// キャッシュ登録完了
			}

			// キャッシュのカバー領域情報をもとに，クラウドへリレーするかを判断する
			// (SWLONがMINLATのLONより小さい and SWLATがMINLONのLATより小さい) or (NELONがMAXLATのLONより大きい and NELATがMAXLONのLATより大きい) -> Global GraphDB
			if (format.SW.Lon < covered_area["MINLAT"][1] && format.SW.Lat < covered_area["MINLON"][0]) || (format.NE.Lon > covered_area["MAXLAT"][1] && format.NE.Lat > covered_area["MAXLON"][0]) {
				// Global GraphDB へリクエスト
				fmt.Println("resolve in global")

				// 2023-06-28 リンクプロセスへの送信
				format.DestSocketAddr = globalGraphDBSockAddr
				// リンクプロセスのソケットアドレスを作成 (クラウドへ投げることは既知)
				linkSrcAddr := link_socket_address_root + "internet_" + server_num + "_0.sock"
				// リンクプロセスへ転送
				connLink, err := net.Dial(protocol, linkSrcAddr)
				if err != nil {
					message.MyError(err, "m2mApi > PointGlobal > net.Dial")
				}
				decoderLink := gob.NewDecoder(connLink)
				encoderLink := gob.NewEncoder(connLink)

				syncFormatClient("Point", decoderLink, encoderLink)

				if err := encoderLink.Encode(format); err != nil {
					message.MyError(err, "m2mApi > PointGlobal > encoderCloud.Encode")
				}
				message.MyWriteMessage(*format)

				// リンクを挟んで Cloud でのポイント解決

				// 受信する型は[]ResolvePoint
				point_output := []m2mapi.ResolvePoint{}
				if err := decoderLink.Decode(&point_output); err != nil {
					message.MyError(err, "m2mApi > PointGlobal > decoderCloud.Decode")
				}
				message.MyReadMessage(point_output)

				// 最終的な結果をM2M Appに送信する
				if err := encoder.Encode(&point_output); err != nil {
					message.MyError(err, "m2mApi > PointGlobal > encoder.Encode")
					break
				}
				message.MyWriteMessage(point_output)
			} else {
				// Local GraphDB へリクエスト
				fmt.Println("resolve in local")
				connDB, err := net.Dial(protocol, graphDBSockAddr)
				if err != nil {
					message.MyError(err, "m2mApi > PointLocal > net.Dial")
				}
				decoderDB := gob.NewDecoder(connDB)
				encoderDB := gob.NewEncoder(connDB)

				syncFormatClient("Point", decoderDB, encoderDB)

				if err := encoderDB.Encode(format); err != nil {
					message.MyError(err, "m2mApi > PointLocal > encoderDB.Encode")
				}
				message.MyWriteMessage(*format)

				// GraphDB()によるDB検索

				// 受信する型は[]ResolvePoint
				point_output := []m2mapi.ResolvePoint{}
				if err := decoderDB.Decode(&point_output); err != nil {
					message.MyError(err, "m2mApi > PointLocal > decoderDB.Decode")
				}
				message.MyReadMessage(point_output)

				// 最終的な結果をM2M Appに送信する
				if err := encoder.Encode(&point_output); err != nil {
					message.MyError(err, "m2mApi > PointLocal > encoder.Encode")
					break
				}
				message.MyWriteMessage(point_output)
			}
		case *m2mapi.ResolveNode:
			format := m2mApiCommand.(*m2mapi.ResolveNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "m2mApi > Node > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// ポイント解決の実行を前提とするので，カバー領域情報のキャッシュは登録済み
			// クラウドへリレーするかの判断
			vpointid_n := "\\\"" + format.VPointID + "\\\""
			local_url := "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_LOCAL_PASSWORD") + "@" + "localhost:" + os.Getenv("NEO4J_LOCAL_PORT_GOLANG") + "/db/data/transaction/commit"
			node_payload := `{"statements": [{"statement": "MATCH (vp: VPoint {VPointID: ` + vpointid_n + `}) RETURN COUNT(vp);"}]}`
			node_datas := listenServer(node_payload, local_url)
			flag := int(node_datas[0].([]interface{})[0].(float64))
			if flag == 0 {
				// Global GraphDB へリクエスト
				fmt.Println("resolve in global")

				// 2023-06-28 リンクプロセスへの送信
				format.DestSocketAddr = globalGraphDBSockAddr
				// リンクプロセスのソケットアドレスを作成 (クラウドへ投げることは既知)
				linkSrcAddr := link_socket_address_root + "internet_" + server_num + "_0.sock"
				// リンクプロセスへ転送
				connLink, err := net.Dial(protocol, linkSrcAddr)
				if err != nil {
					message.MyError(err, "m2mApi > NodeGlobal > net.Dial")
				}
				decoderLink := gob.NewDecoder(connLink)
				encoderLink := gob.NewEncoder(connLink)

				syncFormatClient("Node", decoderLink, encoderLink)

				if err := encoderLink.Encode(format); err != nil {
					message.MyError(err, "m2mApi > NodeGlobal > encoderCloud.Encode")
				}
				message.MyWriteMessage(*format)

				// リンクを挟んで Cloud でのノード解決

				// 受信する型は[]ResolveNode
				node_output := []m2mapi.ResolveNode{}
				if err := decoderLink.Decode(&node_output); err != nil {
					message.MyError(err, "m2mApi > NodeGlobal > decoderCloud.Decode")
				}
				message.MyReadMessage(node_output)

				// 最終的な結果をM2M Appに送信する
				if err := encoder.Encode(&node_output); err != nil {
					message.MyError(err, "m2mApi > NodeGlobal > encoder.Encode")
					break
				}
				message.MyWriteMessage(node_output)
			} else {
				// Local GraphDB へリクエスト
				fmt.Println("resolve in local")
				connDB, err := net.Dial(protocol, graphDBSockAddr)
				if err != nil {
					message.MyError(err, "m2mApi > NodeLocal > net.Dial")
				}
				decoderDB := gob.NewDecoder(connDB)
				encoderDB := gob.NewEncoder(connDB)

				syncFormatClient("Node", decoderDB, encoderDB)

				if err := encoderDB.Encode(format); err != nil {
					message.MyError(err, "m2mApi > NodeLocal > encoderDB.Encode")
				}
				message.MyWriteMessage(*format) //1. 同じ内容

				// GraphDB()によるDB検索

				// 受信する型は[]ResolveNode
				node_output := []m2mapi.ResolveNode{}
				if err := decoderDB.Decode(&node_output); err != nil {
					message.MyError(err, "m2mApi > NodeLocal > decoderDB.Decode")
				}
				message.MyReadMessage(node_output)

				// 最終的な結果をM2M Appに送信する
				if err := encoder.Encode(&node_output); err != nil {
					message.MyError(err, "m2mApi > NodeLocal > encoder.Encode")
					break
				}
				message.MyWriteMessage(node_output)
			}
		case *m2mapi.ResolvePastNode:
			format := m2mApiCommand.(*m2mapi.ResolvePastNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "m2mApi > PastNode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// VNodeスレッドのソケットアドレス
			vsnode_socket := format.SocketAddress

			// リンクプロセスを挟むか否かの分岐
			vsnode_server_num := throughLinkProcess(vsnode_socket)
			if vsnode_server_num != server_num {
				// このサーバに対象のVNodeがない場合，リンクプロセスを噛ます
				format.DestSocketAddr = vsnode_socket
				vsnode_socket = link_socket_address_root + "internet_" + server_num + "_" + vsnode_server_num + ".sock"
			}

			// リンクプロセスを噛ます場合，vsnode_socketはリンクプロセスのソケットアドレス
			connVS, err := net.Dial(protocol, vsnode_socket)
			if err != nil {
				message.MyError(err, "m2mApi > PastNode > net.Dial")
			}
			decoderVS := gob.NewDecoder(connVS)
			encoderVS := gob.NewEncoder(connVS)

			syncFormatClient("PastNode", decoderVS, encoderVS)

			if err := encoderVS.Encode(format); err != nil {
				message.MyError(err, "m2mApi > PastNode > encoderVS.Encode")
			}
			message.MyWriteMessage(*format)

			// VSNodeとのやりとり

			// 受信する型はResolvePastNode
			past_node_output := m2mapi.ResolvePastNode{}
			if err := decoderVS.Decode(&past_node_output); err != nil {
				message.MyError(err, "m2mApi > PastNode > decoderVS.Decode")
			}
			message.MyReadMessage(past_node_output)

			// 最終的な結果をM2M Appに送信する
			if err := encoder.Encode(&past_node_output); err != nil {
				message.MyError(err, "m2mApi > PastNode > encoder.Encode")
				break
			}
			message.MyWriteMessage(past_node_output)
		case *m2mapi.ResolvePastPoint:
			format := m2mApiCommand.(*m2mapi.ResolvePastPoint)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "m2mApi > PastPoint > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// VPointスレッドのソケットアドレス
			vpoint_socket := format.SocketAddress

			// リンクプロセスを挟むか否かの分岐
			vpoint_server_num := throughLinkProcess(vpoint_socket)
			if vpoint_server_num != server_num {
				// このサーバに対象のVPointがない場合，リンクプロセスを噛ます
				format.DestSocketAddr = vpoint_socket
				vpoint_socket = link_socket_address_root + "internet_" + server_num + "_" + vpoint_server_num + ".sock"
			}

			// リンクプロセスを噛ます場合，vpoint_socketはリンクプロセスのソケットアドレス
			connVP, err := net.Dial(protocol, vpoint_socket)
			if err != nil {
				message.MyError(err, "m2mApi > PastPoint > net.Dial")
			}
			decoderVP := gob.NewDecoder(connVP)
			encoderVP := gob.NewEncoder(connVP)

			syncFormatClient("PastPoint", decoderVP, encoderVP)

			if err := encoderVP.Encode(format); err != nil {
				message.MyError(err, "m2mApi > PastPoint > encoderVP.Encode")
			}
			message.MyWriteMessage(*format)

			// VPointとのやりとり

			// 受信する型はResolvePastPoint
			past_point_output := m2mapi.ResolvePastPoint{}
			if err := decoderVP.Decode(&past_point_output); err != nil {
				message.MyError(err, "m2mApi > PastPoint > decoderVP.Decode")
			}
			message.MyReadMessage(past_point_output)

			// 最終的な結果をM2M Appに送信する
			if err := encoder.Encode(&past_point_output); err != nil {
				message.MyError(err, "m2mApi > PastPoint > encoder.Encode")
				break
			}
			message.MyWriteMessage(past_point_output)
		case *m2mapi.ResolveCurrentNode:
			format := m2mApiCommand.(*m2mapi.ResolveCurrentNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "m2mApi > CurrentNode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// VSNodeスレッドのソケットアドレス
			vsnode_socket := format.SocketAddress

			// リンクプロセスを挟むか否かの分岐
			vsnode_server_num := throughLinkProcess(vsnode_socket)
			if vsnode_server_num != server_num {
				// このサーバに対象のVNodeがない場合，リンクプロセスを噛ます
				format.DestSocketAddr = vsnode_server_num
				vsnode_socket = link_socket_address_root + "internet_" + server_num + "_" + vsnode_server_num + ".sock"
			}

			// リンクプロセスを噛ます場合，vsnode_socketはリンクプロセスのソケットアドレス
			connVS, err := net.Dial(protocol, vsnode_socket)
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
			current_node_output := m2mapi.ResolveCurrentNode{}
			if err := decoderVS.Decode(&current_node_output); err != nil {
				message.MyError(err, "m2mApi > CurrentNode > decoderVS.Decode")
			}
			message.MyReadMessage(current_node_output)

			// 最終的な結果をM2M Appに送信する
			if err := encoder.Encode(&current_node_output); err != nil {
				message.MyError(err, "m2mApi > CurrentNode > encoder.Encode")
				break
			}
			message.MyWriteMessage(current_node_output)
		case *m2mapi.ResolveCurrentPoint:
			format := m2mApiCommand.(*m2mapi.ResolveCurrentPoint)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "m2mApi > CurrentPoint > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// 2023-05-26
			// M2M API -> VPoint -> Local GraphDB (VNode検索) -> VPoint -> VNode -> PNode

			// VPointスレッドのソケットアドレス
			vpoint_socket := format.SocketAddress

			// リンクプロセスを挟むか否かの分岐
			vpoint_server_num := throughLinkProcess(vpoint_socket)
			if vpoint_server_num != server_num {
				// このサーバに対象のVPointがない場合，リンクプロセスを噛ます
				format.DestSocketAddr = vpoint_socket
				vpoint_socket = link_socket_address_root + "internet_" + server_num + "_" + vpoint_server_num + ".sock"
			}

			// リンクプロセスをかます場合，vpoint_socketはリンクプロセスのソケットアドレス
			connVP, err := net.Dial(protocol, vpoint_socket)
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
			current_point_output := m2mapi.ResolveCurrentPoint{}
			if err := decoderVP.Decode(&current_point_output); err != nil {
				message.MyError(err, "m2mApi > CurrentPoint > decoderVP.Decode")
			}
			message.MyReadMessage(current_point_output)

			// 最終的な結果をM2M Appに送信する
			if err := encoder.Encode(&current_point_output); err != nil {
				message.MyError(err, "m2mApi > CurrentPoint > encoder.Encode")
				break
			}
			message.MyWriteMessage(current_point_output)
		case *m2mapi.ResolveConditionNode:
			format := m2mApiCommand.(*m2mapi.ResolveConditionNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "m2mApi > ConditionNode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// VSNodeスレッドのソケットアドレス
			vsnode_socket := format.SocketAddress

			// リンクプロセスを挟むか否かの分岐
			vsnode_server_num := throughLinkProcess(vsnode_socket)
			if vsnode_server_num != server_num {
				// このサーバに対象のVNodeがない場合，リンクプロセスを噛ます
				format.DestSocketAddr = vsnode_socket
				vsnode_socket = link_socket_address_root + "internet_" + server_num + "_" + vsnode_server_num + ".sock"
			}

			// リンクプロセスを噛ます場合，vsnode_socketはリンクプロセスのソケットアドレス
			connVS, err := net.Dial(protocol, vsnode_socket)
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
			condition_node_output := m2mapi.DataForRegist{}
			if err := decoderVS.Decode(&condition_node_output); err != nil {
				message.MyError(err, "m2mApi > ConditionNode > decoderVS.Decode")
			}
			message.MyReadMessage(condition_node_output)

			// 最終的な結果をM2M Appに送信する
			if err := encoder.Encode(&condition_node_output); err != nil {
				message.MyError(err, "m2mApi > ConditionNode > encoder.Encode")
				break
			}
			message.MyWriteMessage(condition_node_output)
		case *m2mapi.ResolveConditionPoint:
			format := m2mApiCommand.(*m2mapi.ResolveConditionPoint)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "m2mApi > ConditionPoint > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// VPointスレッドのソケットアドレス
			vpoint_socket := format.SocketAddress

			// リンクプロセスを挟むか否かの分岐
			vpoint_server_num := throughLinkProcess(vpoint_socket)
			if vpoint_server_num != server_num {
				// このサーバに対象のVNodeがない場合，リンクプロセスを噛ます
				format.DestSocketAddr = vpoint_socket
				vpoint_socket = link_socket_address_root + "internet_" + server_num + "_" + vpoint_server_num + ".sock"
			}

			// リンクプロセスを噛ます場合，vpoint_socketはリンクプロセスのソケットアドレス
			connVP, err := net.Dial(protocol, vpoint_socket)
			if err != nil {
				message.MyError(err, "m2mApi > ConditionPoint > net.Dial")
			}
			decoderVP := gob.NewDecoder(connVP)
			encoderVP := gob.NewEncoder(connVP)

			syncFormatClient("ConditionPoint", decoderVP, encoderVP)

			if err := encoderVP.Encode(format); err != nil {
				message.MyError(err, "m2mApi > ConditionPoint > encoderVP.Encode")
			}
			message.MyWriteMessage(*format)
			fmt.Println("Wait for data notification...")

			// VPointからのデータ通知を受ける
			// 受信する型はDataForRegist
			condition_point_output := m2mapi.DataForRegist{}
			if err := decoderVP.Decode(&condition_point_output); err != nil {
				message.MyError(err, "m2mApi > ConditionPoint > decoderVP.Decode")
			}
			message.MyReadMessage(condition_point_output)

			// 最終的な結果をM2M Appに送信する
			if err := encoder.Encode(&condition_point_output); err != nil {
				message.MyError(err, "m2mApi > ConditionPoint > encoder.Encode")
				break
			}
			message.MyWriteMessage(condition_point_output)
		case *m2mapi.Actuate:
			format := m2mApiCommand.(*m2mapi.Actuate)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "m2mApi > Actuate > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			// VSNodeスレッドのソケットアドレス
			vsnode_socket := format.SocketAddress

			// リンクプロセスを挟むか否かの分岐
			vsnode_server_num := throughLinkProcess(vsnode_socket)
			if vsnode_server_num != server_num {
				// このサーバに対象のVNodeがない場合，リンクプロセスを噛ます
				format.DestSocketAddr = vsnode_socket
				vsnode_socket = link_socket_address_root + "internet_" + server_num + "_" + vsnode_server_num + ".sock"
			}

			// リンクプロセスを噛ます場合，vsnode_socketはリンクプロセスのソケットアドレス
			connVS, err := net.Dial(protocol, vsnode_socket)
			if err != nil {
				message.MyError(err, "m2mApi > Actuate > net.Dial")
			}
			decoderVS := gob.NewDecoder(connVS)
			encoderVS := gob.NewEncoder(connVS)

			syncFormatClient("Actuate", decoderVS, encoderVS)

			if err := encoderVS.Encode(format); err != nil {
				message.MyError(err, "m2mApi > Actuate > encoderVS.Encode")
			}
			message.MyWriteMessage(*format)

			// VSNodeからの状態結果を受信する
			// 受信する型はActuate
			actuate_output := m2mapi.Actuate{}
			if err := decoderVS.Decode(&actuate_output); err != nil {
				message.MyError(err, "m2mApi > Actuate > decoderVS.Decode")
			}
			message.MyReadMessage(actuate_output)

			// 最終的な結果をM2M Appに送信する
			if err := encoder.Encode(&actuate_output); err != nil {
				message.MyError(err, "m2mApi > Actuate > encoder.Encode")
				break
			}
			message.MyWriteMessage(actuate_output)
		case string:
			if m2mApiCommand == "exit" {
				// M2MAppでexitが入力されたら，breakする
				break
			}
		}
	}
}

// 仮想モジュールへリクエスト転送する際に，リンクプロセスを挟むか否かの判定
func throughLinkProcess(vsnode_socket string) string {
	dst_server_num_index := strings.Index(vsnode_socket, "_")
	dst_server_num_index_last := strings.LastIndex(vsnode_socket, "_")
	dst_server_num := vsnode_socket[dst_server_num_index+1 : dst_server_num_index_last]
	return dst_server_num
}

// M2M Appと型同期をするための関数
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

// 内部コンポーネント（DB，仮想モジュール）と型同期をするための関数
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
		switch graphDBCommand := syncFormatServer(decoder, encoder); graphDBCommand.(type) {
		case *m2mapi.ResolvePoint:
			format := graphDBCommand.(*m2mapi.ResolvePoint)
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

			payload := `{"statements": [{"statement": "MATCH (ps:PSink)-[:isVirtualizedBy]->(vp:VPoint) WHERE ps.Position[0] > ` + strconv.FormatFloat(swlat, 'f', 4, 64) + ` and ps.Position[1] > ` + strconv.FormatFloat(swlon, 'f', 4, 64) + ` and ps.Position[0] <= ` + strconv.FormatFloat(nelat, 'f', 4, 64) + ` and ps.Position[1] <= ` + strconv.FormatFloat(nelon, 'f', 4, 64) + ` return vp.VPointID, vp.SocketAddress;"}]}`
			var url string
			url = "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_LOCAL_PASSWORD") + "@" + "localhost:" + os.Getenv("NEO4J_LOCAL_PORT_GOLANG") + "/db/data/transaction/commit"
			datas := listenServer(payload, url)

			point_output := []m2mapi.ResolvePoint{}
			for _, data := range datas {
				dataArray := data.([]interface{})
				point := m2mapi.ResolvePoint{}
				point.VPointID = dataArray[0].(string)
				point.SocketAddress = dataArray[1].(string)
				flag := 0
				for _, p := range point_output {
					if p.VPointID == point.VPointID {
						flag = 1
					}
				}
				if flag == 0 {
					point_output = append(point_output, point)
				}
			}

			if err := encoder.Encode(&point_output); err != nil {
				message.MyError(err, "GraphDB > Point > encoder.Encode")
			}
			message.MyWriteMessage(point_output)
		case *m2mapi.ResolveNode:
			format := graphDBCommand.(*m2mapi.ResolveNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "GraphDB > Node > decoder.Decode")
				break
			}
			message.MyReadMessage(format)

			var vpointid_n string
			vpointid_n = "\\\"" + format.VPointID + "\\\""
			capabilities := format.CapsInput
			var format_capabilities []string
			for _, capability := range capabilities {
				capability = "\\\"" + capability + "\\\""
				format_capabilities = append(format_capabilities, capability)
			}
			payload := `{"statements": [{"statement": "MATCH (vp:VPoint {VPointID: ` + vpointid_n + `})-[:aggregates]->(vn:VNode)-[:isPhysicalizedBy]->(pn:PNode) WHERE pn.Capability IN [` + strings.Join(format_capabilities, ", ") + `] return vn.VNodeID, pn.Capability, vn.SocketAddress;"}]}`

			var url string
			url = "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_LOCAL_PASSWORD") + "@" + "localhost:" + os.Getenv("NEO4J_LOCAL_PORT_GOLANG") + "/db/data/transaction/commit"
			datas := listenServer(payload, url)

			node_output := []m2mapi.ResolveNode{}
			for _, data := range datas {
				dataArray := data.([]interface{})
				fmt.Println(dataArray)
				node := m2mapi.ResolveNode{}
				capability := dataArray[1].(string)
				// CapOutputを1つにするか配列にして複数まとめられるようにするか要検討
				//pn.CapOutput = append(pn.CapOutput, capability)
				node.CapOutput = capability
				node.VNodeID = dataArray[0].(string)
				node.SocketAddress = dataArray[2].(string)
				flag := 0
				for _, p := range node_output {
					if p.VNodeID == node.VNodeID {
						flag = 1
					} /*else {
						// CapOutputを1つにするか配列にして複数まとめられるようにするか要検討
						p.Capabilities = append(p.Capabilities, capability)
					}*/
				}
				if flag == 0 {
					node_output = append(node_output, node)
				}
			}

			if err := encoder.Encode(&node_output); err != nil {
				message.MyError(err, "GraphDB > Node > encoder.Encode")
			}
			message.MyWriteMessage(node_output)
		case *server.RequestSessionKey:
			format := graphDBCommand.(*server.RequestSessionKey)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "GraphDB > SessionKey > decoder.Decode")
				break
			}
			message.MyReadMessage(format)

			var pnode_id string
			pnode_id = "\\\"" + format.PNodeID + "\\\""
			payload := `{"statements": [{"statement": "MATCH (pn:PNode {PNodeID: ` + pnode_id + `}) return pn.SessionKey;"}]}`

			var url string
			url = "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_GLOBAL_PASSWORD") + "@" + "localhost:" + os.Getenv("NEO4J_GLOBAL_PORT_GOLANG") + "/db/data/transaction/commit"
			datas := listenServer(payload, url)

			session_key := server.RequestSessionKey{}
			session_key_interface := datas[0].([]interface{})
			session_key.SessionKey = session_key_interface[0].(string)

			if err := encoder.Encode(&session_key); err != nil {
				message.MyError(err, "GraphDB > SessionKey > encoder.Encode")
			}
			message.MyWriteMessage(session_key)
		case *vpoint.CurrentPointVNode:
			// 地点指定型現在データ取得における検索
			format := graphDBCommand.(*vpoint.CurrentPointVNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "GraphDB > CurrentPointVNode > decoder.Decode")
				break
			}
			message.MyReadMessage(format)

			var vpoint_id, capability string
			vpoint_id = "\\\"" + format.VPointID + "\\\""
			capability = "\\\"" + format.Capability + "\\\""
			payload := `{"statements": [{"statement": "MATCH (vp:VPoint {VPointID: ` + vpoint_id + `})-[:aggregates]->(vn:VNode)-[:isPhysicalizedBy]->(pn:PNode {Capability: ` + capability + `}) return vn.SocketAddress, vn.VNodeID;"}]}`

			var url string
			url = "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_LOCAL_PASSWORD") + "@" + "localhost:" + os.Getenv("NEO4J_LOCAL_PORT_GOLANG") + "/db/data/transaction/commit"
			datas := listenServer(payload, url)

			vnode_socket_addresses := vpoint.CurrentPointVNode{}
			for _, v := range datas {
				vnode_interface := v.([]interface{})
				vnode_sock_addr := vnode_interface[0].(string)
				vnode_id := vnode_interface[1].(string)
				vnode_socket_addresses.VNodeSockAddr = append(vnode_socket_addresses.VNodeSockAddr, vnode_sock_addr)
				vnode_socket_addresses.VNodeID = append(vnode_socket_addresses.VNodeID, vnode_id)
			}
			if err := encoder.Encode(&vnode_socket_addresses); err != nil {
				message.MyError(err, "GraphDB > CurrentPointVNode > encoder.Encode")
			}
			message.MyWriteMessage(vnode_socket_addresses)
		}
	}
}

// SensingDB Server
func sensingDB(conn net.Conn) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	//message.MyMessage("[MESSEGE] Call SensingDB thread")

	for {
		// 型同期をして，型の種類に応じてスイッチ
		switch sensingDBCommand := syncFormatServer(decoder, encoder); sensingDBCommand.(type) {
		case *m2mapi.ResolvePastNode:
			format := sensingDBCommand.(*m2mapi.ResolvePastNode)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "SensingDB > PastNode > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			var pnode_id, cap, start, end string
			// 入力のVNodeIDをPNodeIDに変換
			pnode_id = convertID(format.VNodeID, 63)
			cap = format.Capability
			start = format.Period.Start
			end = format.Period.End

			// SensingDBを開く
			// "root:password@tcp(127.0.0.1:3306)/testdb"
			mysql_path := os.Getenv("MYSQL_USERNAME") + ":" + os.Getenv("MYSQL_PASSWORD") + "@tcp(127.0.0.1:" + os.Getenv("MYSQL_PORT") + ")/" + os.Getenv("MYSQL_LOCAL_DB")
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
			DBConnection.SetMaxOpenConns(50)

			var cmd string
			table := os.Getenv("MYSQL_TABLE") + "_" + server_num
			cmd = "SELECT * FROM " + table + " WHERE PNodeID = \"" + pnode_id + "\" AND Capability = \"" + cap + "\" AND Timestamp > \"" + start + "\" AND Timestamp <= \"" + end + "\";"

			rows, err := DBConnection.Query(cmd)
			if err != nil {
				message.MyError(err, "SensingDB > PastNode > DBConnection.Query")
			}
			defer rows.Close()

			sd := m2mapi.ResolvePastNode{}
			for rows.Next() {
				field := []string{"0", "0", "0", "0", "0", "0", "0"}
				// PNodeID, Capability, Timestamp, Value, PSinkID, Lat, Lon
				err := rows.Scan(&field[0], &field[1], &field[2], &field[3], &field[4], &field[5], &field[6])
				if err != nil {
					message.MyError(err, "SensingDB > PastNode > rows.Scan")
				}
				vnode_id := convertID(field[0], 63)
				sd.VNodeID = vnode_id
				valFloat, _ := strconv.ParseFloat(field[3], 64)
				val := m2mapi.Value{Capability: field[1], Time: field[2], Value: valFloat}
				sd.Values = append(sd.Values, val)
			}

			if err := encoder.Encode(&sd); err != nil {
				message.MyError(err, "SensingDB > PastNode > encoder.Encode")
			}
			message.MyWriteMessage(sd)
		case *m2mapi.ResolvePastPoint:
			format := sensingDBCommand.(*m2mapi.ResolvePastPoint)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "SensingDB > PastPoint > decoder.Decode")
				break
			}
			message.MyReadMessage(*format)

			var psink_id, capability, start, end string
			// VPointID_nをPSinkIDに変換
			psink_id = convertID(format.VPointID_n, 63)
			capability = format.Capability
			start = format.Period.Start
			end = format.Period.End

			// SensingDBを開く
			mysql_path := os.Getenv("MYSQL_USERNAME") + ":" + os.Getenv("MYSQL_PASSWORD") + "@tcp(127.0.0.1:" + os.Getenv("MYSQL_PORT") + ")/" + os.Getenv("MYSQL_LOCAL_DB")
			DBConnection, err := sql.Open("mysql", mysql_path)
			if err != nil {
				message.MyError(err, "SensingDB > PastPoint > sql.Open")
			}
			defer DBConnection.Close()
			if err := DBConnection.Ping(); err != nil {
				message.MyError(err, "SensingDB > PastPoint > DBConnection.Ping")
			} else {
				message.MyMessage("DB Connection Success")
			}
			DBConnection.SetMaxOpenConns(50)

			var cmd string
			table := os.Getenv("MYSQL_TABLE") + "_" + server_num
			cmd = "SELECT * FROM " + table + " WHERE PSinkID = \"" + psink_id + "\" AND Capability = \"" + capability + "\" AND Timestamp > \"" + start + "\" AND Timestamp <= \"" + end + "\";"

			rows, err := DBConnection.Query(cmd)
			if err != nil {
				message.MyError(err, "SensingDB > PastPoint > DBConnection.Query")
			}
			defer rows.Close()

			sd := m2mapi.ResolvePastPoint{}
			for rows.Next() {
				field := []string{"0", "0", "0", "0", "0", "0", "0"}
				// PNodeID, Capability, Timestamp, Value, PSinkID, Lat, Lon
				err := rows.Scan(&field[0], &field[1], &field[2], &field[3], &field[4], &field[5], &field[6])
				if err != nil {
					message.MyError(err, "SensingDB > PastPoint > rows.Scan")
				}
				if len(sd.Datas) < 1 {
					vnode_id := convertID(field[0], 63)
					sensordata := m2mapi.SensorData{
						VNodeID_n: vnode_id,
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
						vnode_id := convertID(field[0], 63)
						if vnode_id == data.VNodeID_n {
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
						vnode_id := convertID(field[0], 63)
						sensordata := m2mapi.SensorData{
							VNodeID_n: vnode_id,
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
			format := sensingDBCommand.(*m2mapi.DataForRegist)
			if err := decoder.Decode(format); err != nil {
				if err == io.EOF {
					message.MyMessage("=== closed by client")
					break
				}
				message.MyError(err, "SensingDB > RegisterSensingData > decoder.Decode")
				break
			}
			//message.MyReadMessage(*format)

			var PNodeID, Capability, Timestamp, PSinkID string
			var Value, Lat, Lon float64
			PNodeID = format.PNodeID
			Capability = format.Capability
			Timestamp = format.Timestamp
			Value = format.Value
			PSinkID = format.PSinkID
			Lat = format.Lat
			Lon = format.Lon

			// SensingDBを開く
			mysql_path := os.Getenv("MYSQL_USERNAME") + ":" + os.Getenv("MYSQL_PASSWORD") + "@tcp(127.0.0.1:" + os.Getenv("MYSQL_PORT") + ")/" + os.Getenv("MYSQL_LOCAL_DB")
			DBConnection, err := sql.Open("mysql", mysql_path)
			if err != nil {
				message.MyError(err, "SensingDB > RegisterSensingData > sql.Open")
			}
			defer DBConnection.Close()
			if err := DBConnection.Ping(); err != nil {
				message.MyError(err, "SensingDB > RegisterSensingData > DBConnection.Ping")
			} /*else {
				message.MyMessage("DB Connection Success")
			}*/
			//DBConnection.SetMaxOpenConns(500)

			// PNodeID, Capability, Timestamp, Value, PSinkID, Lat, Lon
			var cmd string
			table := os.Getenv("MYSQL_TABLE") + "_" + server_num
			cmd = "INSERT INTO " + table + "(PNodeID,Capability,Timestamp,Value,PSinkID,Lat,Lon) VALUES(?,?,?,?,?,?,?);"

			in, err := DBConnection.Prepare(cmd)
			if err != nil {
				message.MyError(err, "SensingDB > RegisterSensingData > DBConnection.Prepare")
			}
			if _, errExec := in.Exec(PNodeID, Capability, Timestamp, Value, PSinkID, Lat, Lon); errExec == nil {
				//message.MyMessage("Complete Data Registration!")
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

func loadEnv() {
	// .envファイルの読み込み
	if err := godotenv.Load(os.Getenv("HOME") + "/.env"); err != nil {
		message.MyError(err, "loadEnv > godotenv.Load")
	}
}
