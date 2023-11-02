package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"mecm2m-Emulator/pkg/message"
	"mecm2m-Emulator/pkg/vsnode"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

const (
	protocol                         = "unix"
	layout                           = "2006-01-02 15:04:05 +0900 JST"
	timeSock                         = "/tmp/mecm2m/time.sock"
	dataResisterSock                 = "/tmp/mecm2m/data_resister.sock"
	socket_address_root              = "/tmp/mecm2m/"
	link_process_socket_address_path = "/tmp/mecm2m/link-process"
	concurrency                      = 3600
)

type Format struct {
	FormType string
}

type Ports struct {
	Port []int `json:"ports"`
}

type CurrentTime struct {
}

var (
	currentTime          CurrentTime
	data_resister_socket string
	mu                   sync.Mutex
)

func init() {
	// .envファイルの読み込み
	if err := godotenv.Load(os.Getenv("HOME") + "/.env"); err != nil {
		log.Fatal(err)
	}
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

func resolveCurrentNode(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolveCurrentNode: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &vsnode.ResolveCurrentDataByNode{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolveCurrentNode: Error missmatching packet format", http.StatusInternalServerError)
		}

		randomFloat := randomFloat64()
		min := 30.0
		//max := 40.0
		value_value := min + randomFloat

		results := vsnode.ResolveCurrentDataByNode{
			PNodeID:    inputFormat.PNodeID,
			Capability: inputFormat.Capability,
			Value:      value_value,
			Timestamp:  time.Now().Format(layout),
		}

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

func actuate(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "actuate: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &vsnode.Actuate{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "actuate: Error missmatching packet format", http.StatusInternalServerError)
		}

		// アクチュエートの内容をファイルに記載したい
		url := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/PSNode/actuate.txt"
		file, err := os.Create(url)
		if err != nil {
			fmt.Println("Error creating actuate file")
			return
		}
		defer file.Close()

		// fileに書き込むためのWriter
		writer := bufio.NewWriter(file)
		mu.Lock()
		fmt.Fprintf(writer, "Lock\n")
		fmt.Fprintf(writer, "VNodeID: %v,Capability: %v, Action: %v, Parameter: %v\n", inputFormat.PNodeID, inputFormat.Capability, inputFormat.Action, inputFormat.Parameter)
		fmt.Fprintf(writer, "Unlock\n")
		err = writer.Flush()
		mu.Unlock()

		status := true
		if err != nil {
			status = false
		}
		results := vsnode.Actuate{
			Status: status,
		}

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

func startServer(port int) {
	mux := http.NewServeMux() // 新しいServeMuxインスタンスを作成
	mux.HandleFunc("/devapi/data/current/node", resolveCurrentNode)
	mux.HandleFunc("/devapi/actuate", actuate)

	address := fmt.Sprintf(":%d", port)
	// log.Printf("Starting server on %s", address)

	server := &http.Server{
		Addr:    address,
		Handler: mux,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Error starting server on port %d: %v", port, err)
	}
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

	// 初期環境構築時に作成したPSNodeのポート分だけ必要
	initial_environment_file := os.Getenv("HOME") + os.Getenv("PROJECT_PATH") + "/PSNode/initial_environment.json"
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

	defer close(sem)
	wg.Wait()
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

func getGID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}

func trimVSNodePort(address string) string {
	host := strings.Split(address, ":")

	var port int
	if len(host) > 1 {
		port, _ = strconv.Atoi(host[1])
	} else {
		return ""
	}
	base_port, _ := strconv.Atoi(os.Getenv("PSNODE_BASE_PORT"))
	pnode_id_index := port - base_port
	pnode_id_int := (0b0010 << 60) + pnode_id_index
	pnode_id := strconv.Itoa(pnode_id_int)

	return pnode_id
}

func randomFloat64() float64 {
	n, err := rand.Int(rand.Reader, big.NewInt(1000))
	if err != nil {
		panic(err)
	}
	floatValue := new(big.Float).SetInt(n)
	float64Value, _ := floatValue.Float64()
	f := float64Value / 100
	return f
}
