package main

import (
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"mecm2m-Emulator/pkg/m2mapi"
	"mecm2m-Emulator/pkg/message"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/joho/godotenv"

	_ "github.com/go-sql-driver/mysql"
)

const (
	protocol                         = "unix"
	link_process_socket_address_path = "/tmp/mecm2m/link-process"
)

type Format struct {
	FormType string
}

func resolvePastNode(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolvePastNode: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ResolvePastNode{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolvePastNode: Error missmatching packet format", http.StatusInternalServerError)
		}

		// 入力のVNodeIDをPNodeIDに変換
		pnode_id := convertID(inputFormat.VNodeID, 63)
		capability := inputFormat.Capability
		start := inputFormat.Period.Start
		end := inputFormat.Period.End

		// SensingDBを開く
		mysql_path := os.Getenv("MYSQL_USERNAME") + ":" + os.Getenv("MYSQL_PASSWORD") + "@tcp(127.0.0.1:" + os.Getenv("MYSQL_PORT") + ")/" + os.Getenv("MYSQL_LOCAL_DB")
		DBConnection, err := sql.Open("mysql", mysql_path)
		if err != nil {
			http.Error(w, "resolvePastNode: Error opening SensingDB", http.StatusInternalServerError)
		}
		defer DBConnection.Close()
		if err := DBConnection.Ping(); err != nil {
			http.Error(w, "resolvePastNode: Error connecting SensingDB", http.StatusInternalServerError)
		} else {
			message.MyMessage("DB Connection Success")
		}
		// DBへの同時接続数の制限
		//DBConnection.SetMaxOpenConns(50)

		var cmd string
		table := os.Getenv("MYSQL_TABLE")
		cmd = "SELECT * FROM " + table + " WHERE PNodeID = \"" + pnode_id + "\" AND Capability = \"" + capability + "\" AND Timestamp > \"" + start + "\" AND Timestamp <= \"" + end + "\";"

		rows, err := DBConnection.Query(cmd)
		if err != nil {
			http.Error(w, "resolvePastNode: Error querying SensingDB", http.StatusInternalServerError)
		}
		defer rows.Close()

		var vnode_id string
		var vals []m2mapi.Value
		for rows.Next() {
			field := []string{"0", "0", "0", "0", "0", "0", "0"}
			// PNodeID, Capability, Timestamp, Value, PSinkID, Lat, Lon
			err := rows.Scan(&field[0], &field[1], &field[2], &field[3], &field[4], &field[5], &field[6])
			if err != nil {
				http.Error(w, "resolvePastNode: Error scanning sensing data", http.StatusInternalServerError)
			}
			vnode_id = convertID(field[0], 63)
			valFloat, _ := strconv.ParseFloat(field[3], 64)
			val := m2mapi.Value{Capability: field[1], Time: field[2], Value: valFloat}
			vals = append(vals, val)
		}
		result := m2mapi.ResolvePastNode{
			VNodeID: vnode_id,
			Values:  vals,
		}

		jsonData, err := json.Marshal(result)
		if err != nil {
			http.Error(w, "resolvePastNode: Error marshaling data", http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "%v\n", string(jsonData))
	} else {
		http.Error(w, "resolvePastNode: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func resolveCurrentNode(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolvePastNode: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ResolvePastNode{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolvePastNode: Error missmatching packet format", http.StatusInternalServerError)
		}

		// リンクプロセスを噛ます
		pnode_id := convertID(inputFormat.VNodeID, 63)
		link_process_socket_address := link_process_socket_address_path + "/access-network_" + pnode_id + ".sock"
		connLinkProcess, err := net.Dial(protocol, link_process_socket_address)
		if err != nil {
			message.MyError(err, "resolveCurrentNode > net.Dial")
		}
		decoderLinkProcess := gob.NewDecoder(connLinkProcess)
		encoderLinkProcess := gob.NewEncoder(connLinkProcess)

		syncFormatClient("CurrentNode", decoderLinkProcess, encoderLinkProcess)

		if err := encoderLinkProcess.Encode(inputFormat); err != nil {
			message.MyError(err, "resolveCurrentNode > encoderLinkProcess.Encode")
		}

		// PSNodeへ

		// 受信する型は m2mapi.ResolveCurrentNode
		results := m2mapi.ResolveCurrentNode{}
		if err := decoderLinkProcess.Decode(&results); err != nil {
			message.MyError(err, "resolveCurrentNode > decoderLinkProcess.Decode")
		}

		// 最後にM2M APIへ返送
		jsonData, err := json.Marshal(results)
		if err != nil {
			http.Error(w, "resolveCurrentNode: Error marshaling data", http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "%v\n", string(jsonData))
	} else {
		http.Error(w, "resolveCurrentNode: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func startServer(port int) {
	mux := http.NewServeMux() // 新しいServeMuxインスタンスを作成
	mux.HandleFunc("/primapi/data/past/node", resolvePastNode)
	mux.HandleFunc("/primapi/data/current/node", resolveCurrentNode)

	address := fmt.Sprintf(":%d", port)
	log.Printf("Starting server on %s", address)

	server := &http.Server{
		Addr:    address,
		Handler: mux,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Error starting server on port %d: %v", port, err)
	}
}

type Ports struct {
	Port []int `json:"ports"`
}

func main() {
	loadEnv()
	var wg sync.WaitGroup

	// 初期環境構築時に作成したVSNodeのポート分だけ必要
	initial_environment_file := os.Getenv("HOME") + os.Getenv("PROJECT_PATH") + "/VSNode/initial_environment.json"
	file, err := os.Open(initial_environment_file)
	if err != nil {
		fmt.Println("Error opening file: ", err)
		return
	}
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	if err != nil {
		fmt.Println("Error reading file: ", err)
		return
	}

	var ports Ports
	err = json.Unmarshal(data, &ports)
	if err != nil {
		fmt.Println("Error decoding JSON: ", err)
		return
	}

	for _, port := range ports.Port {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			startServer(port)
		}(port)
	}

	wg.Wait()
}

func syncFormatClient(command string, decoder *gob.Decoder, encoder *gob.Encoder) {
	format := &Format{}
	switch command {
	case "CurrentNode":
		format.FormType = "CurrentNode"
	}
	if err := encoder.Encode(format); err != nil {
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

func loadEnv() {
	// .envファイルの読み込み
	if err := godotenv.Load(os.Getenv("HOME") + "/.env"); err != nil {
		//message.MyError(err, "loadEnv > godotenv.Load")
		log.Fatal(err)
	}
}
