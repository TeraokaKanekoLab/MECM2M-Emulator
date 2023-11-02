package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"mecm2m-Emulator/pkg/message"
	"mecm2m-Emulator/pkg/psnode"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const layout = "2006-01-02 15:04:05 +0900 JST"

type Ports struct {
	Port []int `json:"ports"`
}

func main() {
	loadEnv()

	vsnode_initial_environment_file := os.Getenv("HOME") + os.Getenv("PROJECT_PATH") + "/VSNode/initial_environment.json"
	file, err := os.Open(vsnode_initial_environment_file)
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

	var vsnode_ports Ports
	err = json.Unmarshal(data, &vsnode_ports)
	if err != nil {
		fmt.Println("Error decoding JSON: ", err)
		return
	}

	vsnode_ports.Port = []int{11000}
	for {
		for _, port := range vsnode_ports.Port {
			port_str := strconv.Itoa(port)
			pnode_id := trimPNodeID(port)
			send_data := psnode.TimeSync{
				PNodeID:     pnode_id,
				CurrentTime: time.Now(),
			}
			sensordata := generateSensordata(&send_data)
			url := "http://localhost:" + port_str + "/data/register"
			client_data, err := json.Marshal(sensordata)
			if err != nil {
				fmt.Println("Error marshaling data: ", err)
				return
			}
			response, err := http.Post(url, "application/json", bytes.NewBuffer(client_data))
			if err != nil {
				fmt.Println("Error making request: ", err)
				return
			}
			defer response.Body.Close()
			time.Sleep(1 * time.Second)
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

func trimPNodeID(port int) string {
	base_port, _ := strconv.Atoi(os.Getenv("VSNODE_BASE_PORT"))
	id_index := port - base_port
	pnode_id_int := int(0b0010<<60) + id_index
	pnode_id := strconv.Itoa(pnode_id_int)
	return pnode_id
}

// センサデータの登録
func generateSensordata(inputFormat *psnode.TimeSync) psnode.DataForRegist {
	var result psnode.DataForRegist
	// PSNodeのconfigファイルを検索し，ソケットファイルと一致する情報を取得する
	psnode_json_file_path := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/setup/GraphDB/config/config_main_psnode.json"
	psnodeJsonFile, err := os.Open(psnode_json_file_path)
	if err != nil {
		fmt.Println(err)
	}
	defer psnodeJsonFile.Close()
	psnodeByteValue, _ := io.ReadAll(psnodeJsonFile)

	var psnodeResult map[string][]interface{}
	json.Unmarshal(psnodeByteValue, &psnodeResult)

	psnodes := psnodeResult["psnodes"]
	for _, v := range psnodes {
		psnode_format := v.(map[string]interface{})
		psnode := psnode_format["psnode"].(map[string]interface{})
		//psnode_relation_label := psnode["relation-label"].(map[string]interface{})
		psnode_data_property := psnode["data-property"].(map[string]interface{})
		pnode_id := psnode_data_property["PNodeID"].(string)
		if pnode_id == inputFormat.PNodeID {
			result.PNodeID = pnode_id
			result.Capability = psnode_data_property["Capability"].(string)
			result.Timestamp = inputFormat.CurrentTime.Format(layout)
			randomFloat := randomFloat64()
			min := 30.0
			//max := 40.0
			value_value := min + randomFloat
			result.Value = value_value
			//result.PSinkID = psnode_relation_label["PSink"].(string)
			result.PSinkID = "PSink"
			position := psnode_data_property["Position"].([]interface{})
			result.Lat = position[0].(float64)
			result.Lon = position[1].(float64)
		}
	}
	return result
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
	byteArray, _ := io.ReadAll(resp.Body)

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
