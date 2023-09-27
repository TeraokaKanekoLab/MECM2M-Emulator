package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mecm2m-Emulator/pkg/m2mapp"
	"mecm2m-Emulator/pkg/message"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

func main() {
	loadEnv()

	jsonStr := m2mapp.ResolveAreaInput{
		NE: m2mapp.SquarePoint{Lat: 35.533, Lon: 139.532},
		SW: m2mapp.SquarePoint{Lat: 35.531, Lon: 139.53},
	}

	jsonStr_byte, _ := json.Marshal(jsonStr)

	// JSONデコード用のマップ
	var data map[string]interface{}

	// JSONデコード
	if err := json.Unmarshal(jsonStr_byte, &data); err != nil {
		fmt.Println(err)
		return
	}

	// 各フィールドごとに処理
	for key, value := range data {
		switch key {
		case "field1":
			// フィールド1に対する処理
			fmt.Println("Field1:", value.(string))
		case "field2":
			// フィールド2に対する処理
			fmt.Println("Field2:", value.([]interface{}))
		case "field3":
			// フィールド3に対する処理
			if bytesData, ok := value.([]byte); ok {
				// []byte型として処理
				fmt.Println("Field3 (as []byte):", string(bytesData))
			} else {
				fmt.Println("Field3 (not []byte):", value)
			}
		default:
			// その他のフィールドに対する処理
			fmt.Println("Unknown field:", key)
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
