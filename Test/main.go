package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mecm2m-Simulator/pkg/message"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

// 自身のカバー領域情報をキャッシュ
var covered_area = make(map[string][]float64)
var mu sync.Mutex

func main() {
	loadEnv()

	local_url := "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_LOCAL_PASSWORD") + "@" + "localhost:" + os.Getenv("NEO4J_LOCAL_PORT_GOLANG") + "/db/data/transaction/commit"
	// エッジサーバのカバー領域に関するキャッシュ情報の取得
	mu.Lock()
	if _, ok := covered_area["MINLAT"]; !ok {
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
			max_lat_interface := max_lat_record["SW"].([]interface{})
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
			max_lon_interface := max_lon_record["SW"].([]interface{})
			var max_lon []float64
			for _, v := range max_lon_interface {
				max_lon = append(max_lon, v.(float64))
			}
			covered_area["MAXLON"] = max_lon
		}
	}
	mu.Unlock()

	fmt.Println(covered_area)

	// 指定された矩形範囲が少しでもカバー領域から外れていれば，クラウドサーバへリレー
	if 35.531 > covered_area["MINLON"][0] && 139.531 > covered_area["MINLAT"][1] && 35.532 <= covered_area["MAXLON"][0] && 139.532 <= covered_area["MAXLAT"][1] {
		// すべての領域がカバーエリア内
	} else {
		// クラウドサーバへ
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
