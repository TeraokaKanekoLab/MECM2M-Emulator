package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"mecm2m-Emulator/pkg/m2mapi"
	"mecm2m-Emulator/pkg/message"
	"mecm2m-Emulator/pkg/psnode"
	"mecm2m-Emulator/pkg/vmnode"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/joho/godotenv"

	_ "github.com/go-sql-driver/mysql"
)

const (
	protocol                         = "unix"
	link_process_socket_address_path = "/tmp/mecm2m/link-process"
	concurrency                      = 3600
)

type Format struct {
	FormType string
}

var (
	// 充足条件データ取得用のセンサデータのバッファ．(key, value) = (PNodeID, DataForRegist)
	bufferSensorData = make(map[string]psnode.DataForRegist)
	mu               sync.Mutex

	// センサデータがバッファされたときに通知するチャネル
	buffer_chan = make(chan string)
)

func init() {
	// .envファイルの読み込み
	if err := godotenv.Load(os.Getenv("HOME") + "/.env"); err != nil {
		log.Fatal(err)
	}
}

func resolvePastNode(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolvePastNode: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ResolveDataByNode{}
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
		defer DBConnection.Close()
		// DBへの同時接続数の制限
		//DBConnection.SetMaxOpenConns(50)

		var cmd string
		table := os.Getenv("MYSQL_TABLE")
		var format_capability []string
		for _, cap := range capability {
			cap = "\"" + cap + "\""
			format_capability = append(format_capability, cap)
		}
		cmd = "SELECT * FROM " + table + " WHERE PNodeID = \"" + pnode_id + "\" AND Capability IN (" + strings.Join(format_capability, ",") + ") AND Timestamp > \"" + start + "\" AND Timestamp <= \"" + end + "\";"

		rows, err := DBConnection.Query(cmd)
		if err != nil {
			http.Error(w, "resolvePastNode: Error querying SensingDB", http.StatusInternalServerError)
		}
		defer rows.Close()

		// vmnodeが受信する型として情報を持たせておく
		var sensing_db_results []vmnode.ResolveDataByNode
		for rows.Next() {
			field := []string{"0", "0", "0", "0", "0", "0", "0"}
			// PNodeID, Capability, Timestamp, Value, PSinkID, Lat, Lon
			err := rows.Scan(&field[0], &field[1], &field[2], &field[3], &field[4], &field[5], &field[6])
			if err != nil {
				http.Error(w, "resolvePastNode: Error scanning sensing data", http.StatusInternalServerError)
			}
			value_float, _ := strconv.ParseFloat(field[3], 64)
			lat_float, _ := strconv.ParseFloat(field[5], 64)
			lon_float, _ := strconv.ParseFloat(field[6], 64)
			sensing_db_result := vmnode.ResolveDataByNode{
				PNodeID:    field[0],
				Capability: field[1],
				Timestamp:  field[2],
				Value:      value_float,
				Lat:        lat_float,
				Lon:        lon_float,
			}
			sensing_db_results = append(sensing_db_results, sensing_db_result)
		}

		var vnode_id string
		var vals []m2mapi.Value
		for i, sensing_db_result := range sensing_db_results {
			if i == 0 {
				vnode_id = convertID(sensing_db_result.PNodeID, 63)
			}
			val := m2mapi.Value{
				Capability: sensing_db_result.Capability,
				Time:       sensing_db_result.Timestamp,
				Value:      sensing_db_result.Value,
			}
			vals = append(vals, val)
		}
		result := m2mapi.ResolveDataByNode{
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
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolveCurrentNode: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ResolveDataByNode{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolveCurrentNode: Error missmatching packet format", http.StatusInternalServerError)
		}

		// PSNodeへリクエストを送信するためにリンクプロセスを噛ます
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

		// PMNode (VMNodeR) へ

		// 受信する型は m2mapi.ResolveDataByNode
		results := m2mapi.ResolveDataByNode{}
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

func resolveConditionNode(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolveConditionNode: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ResolveDataByNode{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolveConditionNode: Error missmatching packet format", http.StatusInternalServerError)
		}

		// PSNodeからの定期的なセンサデータ登録で受信するセンサデータを読み込み，Conditionと合致する内容であれば，M2M APIへ返送する
		inputPNodeID := convertID(inputFormat.VNodeID, 63, 61)
		buffer_data := bufferSensorData[inputPNodeID]
		val := buffer_data.Value

		lowerLimit := inputFormat.Condition.Limit.LowerLimit
		upperLimit := inputFormat.Condition.Limit.UpperLimit
		timeout := inputFormat.Condition.Timeout

		timeoutContext, cancelFunc := context.WithTimeout(context.Background(), timeout)
		defer cancelFunc()

		fmt.Println("Wait for Condition Data...")
	Loop:
		for {
			select {
			case <-timeoutContext.Done():
				fmt.Println("Timeout Deadline")
				nullData := m2mapi.ResolveDataByNode{
					VNodeID: "Timeout",
				}
				jsonData, err := json.Marshal(nullData)
				if err != nil {
					http.Error(w, "resolveConditionNode: Error marshaling data", http.StatusInternalServerError)
					break Loop
				}
				fmt.Fprintf(w, "%v\n", string(jsonData))
				return
			case <-buffer_chan:
				mu.Lock()
				if val != bufferSensorData[inputPNodeID].Value {
					// バッファデータ更新
					val = bufferSensorData[inputPNodeID].Value
				}
				mu.Unlock()

				if val >= lowerLimit && val < upperLimit {
					// 条件を満たすので，M2M APIへ結果を転送
					register_data := bufferSensorData[inputPNodeID]
					values := []m2mapi.Value{}
					value := m2mapi.Value{
						Capability: register_data.Capability,
						Time:       register_data.Timestamp,
						Value:      register_data.Value,
					}
					values = append(values, value)
					data := m2mapi.ResolveDataByNode{
						Values: values,
					}
					jsonData, err := json.Marshal(data)
					if err != nil {
						http.Error(w, "resolveConditionNode: Error marshaling data", http.StatusInternalServerError)
						return
					}
					fmt.Fprintf(w, "%v\n", string(jsonData))
					bufferSensorData[inputPNodeID] = psnode.DataForRegist{}
					return
				} else {
					continue Loop
				}
			}
		}

	} else {
		http.Error(w, "resolveConditionNode: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func actuate(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "actuate: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.Actuate{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "actuate: Error missmatching packet format", http.StatusInternalServerError)
		}

		// PSNodeへリクエストを送信するためにリンクプロセスを噛ます
		pnode_id := convertID(inputFormat.VNodeID, 63, 61)
		link_process_socket_address := link_process_socket_address_path + "/access-network_" + pnode_id + ".sock"
		connLinkProcess, err := net.Dial(protocol, link_process_socket_address)
		if err != nil {
			message.MyError(err, "resolveCurrentNode > net.Dial")
		}
		decoderLinkProcess := gob.NewDecoder(connLinkProcess)
		encoderLinkProcess := gob.NewEncoder(connLinkProcess)

		syncFormatClient("Actuate", decoderLinkProcess, encoderLinkProcess)

		if err := encoderLinkProcess.Encode(inputFormat); err != nil {
			message.MyError(err, "resolveCurrentNode > encoderLinkProcess.Encode")
		}

		// PSNodeへ

		// 受信する型は m2mapi.Actuate
		results := m2mapi.Actuate{}
		if err := decoderLinkProcess.Decode(&results); err != nil {
			message.MyError(err, "actuate > decoderLinkProcess.Decode")
		}

		// 最後にM2M APIへ返送
		jsonData, err := json.Marshal(results)
		if err != nil {
			http.Error(w, "actuate: Error marshaling data", http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "%v\n", string(jsonData))
	} else {
		http.Error(w, "actuate: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func dataRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "dataRegister: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &psnode.DataForRegist{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "dataRegister: Error missmatching packet format", http.StatusInternalServerError)
		}

		// Local GraphDB に対して，VSNode が PSNode のセッションキーをキャッシュしていない場合に聞きに行く工程がある

		// SensingDBを開く
		mysql_path := os.Getenv("MYSQL_USERNAME") + ":" + os.Getenv("MYSQL_PASSWORD") + "@tcp(127.0.0.1:" + os.Getenv("MYSQL_PORT") + ")/" + os.Getenv("MYSQL_LOCAL_DB")
		DBConnection, err := sql.Open("mysql", mysql_path)
		if err != nil {
			http.Error(w, "dataRegister: Error opening SensingDB", http.StatusInternalServerError)
		}
		defer DBConnection.Close()
		if err := DBConnection.Ping(); err != nil {
			http.Error(w, "dataRegister: Error connecting SensingDB", http.StatusInternalServerError)
		} else {
			message.MyMessage("DB Connection Success")
		}
		defer DBConnection.Close()
		// DBへの同時接続数の制限
		//DBConnection.SetMaxOpenConns(50)

		// データの挿入
		var cmd string
		table := os.Getenv("MYSQL_TABLE")
		cmd = "INSERT INTO " + table + "(PNodeID, Capability, Timestamp, Value, PSinkID, Lat, Lon) VALUES(?, ?, ?, ?, ?, ?, ?)"
		//cmd = "INSERT INTO " + table + "(PNodeID, Capability, Timestamp) VALUES(?, ?, ?)"
		stmt, err := DBConnection.Prepare(cmd)
		if err != nil {
			http.Error(w, "dataRegister: Error preparing SensingDB", http.StatusInternalServerError)
		}
		defer stmt.Close()

		_, err = stmt.Exec(inputFormat.PNodeID, inputFormat.Capability, inputFormat.Timestamp, inputFormat.Value, inputFormat.PSinkID, inputFormat.Lat, inputFormat.Lon)
		if err != nil {
			http.Error(w, "dataRegister: Error exec SensingDB", http.StatusInternalServerError)
		}

		// バッファにセンサデータ登録
		mu.Lock()
		registerPNodeID := inputFormat.PNodeID
		bufferSensorData[registerPNodeID] = *inputFormat
		mu.Unlock()

		// チャネルに知らせる
		go func(buffer_chan chan string) {
			transmit_string := registerPNodeID
			buffer_chan <- transmit_string
		}(buffer_chan)

		fmt.Println("Data Inserted Successfully!")
		fmt.Fprintf(w, "%v\n", "Register Success")
	} else {
		http.Error(w, "dataRegister: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func mobility(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "mobility: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &psnode.Mobility{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "mobility: Error missmatching packet format", http.StatusInternalServerError)
		}
		pnode_id := "\\\"" + inputFormat.PNodeID + "\\\""

		payload := `{"statements": [{"statement": "MATCH (pmn:PMNode {PNodeID: ` + pnode_id + `}) return pmn.Position;"}]}`
		graphdb_url := "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_LOCAL_PASSWORD") + "@localhost:" + os.Getenv("NEO4J_LOCAL_PORT_GOLANG") + "/db/data/transaction/commit"
		req, _ := http.NewRequest("POST", graphdb_url, bytes.NewBuffer([]byte(payload)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "*/*")

		client := new(http.Client)
		resp, err := client.Do(req)
		if err != nil {
			message.MyError(err, "resolvePointFunction > client.Do()")
		}
		defer resp.Body.Close()

		byteArray, _ := io.ReadAll(resp.Body)
		values := bodyGraphDB(byteArray)

		var row_data interface{}
		var position [2]float64
		for _, v1 := range values {
			for k2, v2 := range v1.(map[string]interface{}) {
				if k2 == "data" {
					for _, v3 := range v2.([]interface{}) {
						for k4, v4 := range v3.(map[string]interface{}) {
							if k4 == "row" {
								row_data = v4
								dataArray := row_data.([]interface{})
								a := dataArray[0].([]interface{})
								position[0] = a[0].(float64)
								position[1] = a[1].(float64)
							}
						}
					}
				}
			}
		}
		position[0] += 0.001

		// 自MEC ServerのLocal GraphDBへの検索
		payload = `{"statements": [{"statement": "MATCH (pmn:PMNode {PNodeID: ` + pnode_id + `}) SET pmn.Position = [` + strconv.FormatFloat(position[0], 'f', 4, 64) + `, ` + strconv.FormatFloat(position[1], 'f', 4, 64) + `] return pmn.Position;"}]}`
		graphdb_url = "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_LOCAL_PASSWORD") + "@localhost:" + os.Getenv("NEO4J_LOCAL_PORT_GOLANG") + "/db/data/transaction/commit"
		req, _ = http.NewRequest("POST", graphdb_url, bytes.NewBuffer([]byte(payload)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "*/*")

		client = new(http.Client)
		resp, err = client.Do(req)
		if err != nil {
			message.MyError(err, "resolvePointFunction > client.Do()")
		}
		defer resp.Body.Close()
	} else {
		http.Error(w, "dataRegister: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func startServer(port int) {
	mux := http.NewServeMux() // 新しいServeMuxインスタンスを作成
	mux.HandleFunc("/primapi/data/past/node", resolvePastNode)
	mux.HandleFunc("/primapi/data/current/node", resolveCurrentNode)
	mux.HandleFunc("/primapi/data/condition/node", resolveConditionNode)
	mux.HandleFunc("/primapi/actuate", actuate)
	mux.HandleFunc("/data/register", dataRegister)
	mux.HandleFunc("/mobility", mobility)

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
	/*
		// Mainプロセスのコマンドラインからシミュレーション実行開始シグナルを受信するまで待機
		signals_from_main := make(chan os.Signal, 1)

		// 停止しているプロセスを再開するために送信されるシグナル，SIGCONT(=18)を受信するように設定
		signal.Notify(signals_from_main, syscall.SIGCONT)

		// シグナルを待機
		fmt.Println("Waiting for signal...")
		sig := <-signals_from_main

		// 受信したシグナルを表示
		fmt.Printf("Received signal: %v\n", sig)
	*/
	var wg sync.WaitGroup

	// 初期環境構築時に作成したVSNodeのポート分だけ必要
	initial_environment_file := os.Getenv("HOME") + os.Getenv("PROJECT_PATH") + "/VMNode/initial_environment.json"
	file, err := os.Open(initial_environment_file)
	if err != nil {
		fmt.Println("Error opening file: ", err)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
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

	sem := make(chan struct{}, concurrency)
	fmt.Println("Starting Server")

	for _, port := range ports.Port {
		sem <- struct{}{}

		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			defer func() { <-sem }()
			startServer(port)
		}(port)
	}

	// 別の goroutine で上のすべての goroutine が終わるまで待機
	// 終了したら，チャネルをclose
	defer close(sem)
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

func convertID(id string, pos ...int) string {
	id_int := new(big.Int)

	_, ok := id_int.SetString(id, 10)
	if !ok {
		message.MyMessage("Failed to convert string to big.Int")
	}

	for _, position := range pos {
		mask := new(big.Int).Lsh(big.NewInt(1), uint(position))
		id_int.Xor(id_int, mask)
	}
	return id_int.String()
}

func bodyGraphDB(byteArray []byte) []interface{} {
	var jsonBody map[string]interface{}
	if err := json.Unmarshal(byteArray, &jsonBody); err != nil {
		message.MyError(err, "bodyGraphDB > json.Unmarshal")
		return nil
	}
	var values []interface{}
	for _, v1 := range jsonBody {
		switch v1.(type) {
		case []interface{}:
			for range v1.([]interface{}) {
				values = v1.([]interface{})
			}
		case map[string]interface{}:
			for _, v2 := range v1.(map[string]interface{}) {
				switch v2.(type) {
				case []interface{}:
					values = v2.([]interface{})
				default:
				}
			}
		default:
			fmt.Println("Format Assertion False")
		}
	}
	return values
}
