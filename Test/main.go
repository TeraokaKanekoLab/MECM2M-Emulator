package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mecm2m-Simulator/pkg/message"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	loadEnv()
	// file_nameからサーバ番号を抽出
	file_name := "/tmp/mecm2m/link-process/internet_0_1.sock"
	src_index := strings.Index(file_name, "_")
	dst_index := strings.LastIndex(file_name, "_")
	dot_index := strings.LastIndex(file_name, ".")
	src_server_num := file_name[src_index+1 : dst_index]
	dst_server_num := file_name[dst_index+1 : dot_index]
	fmt.Println(src_server_num, dst_server_num)
	// RTT時間の検索
	rtt_fp, err := os.Open("../LinkProcess/CloudMEC/rtt.csv")
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
			rtt_str := record[2] + "s"
			rtt, _ := time.ParseDuration(rtt_str)
			time.Sleep(rtt)
		}
	}
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
