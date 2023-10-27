package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mecm2m-Emulator/pkg/message"
	"mecm2m-Emulator/pkg/psnode"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	loadEnv()
	start_time := time.Now()

	/*
		for k := 0; k < 1; k++ {
			var wg sync.WaitGroup
			for block := 0; block < 24; block++ {
				start := 21000 + (block * 150)
				end := start + 150
				for i := start; i < end; i++ {
					wg.Add(1)
					go func(port int) {
						defer wg.Done()
						port_str := strconv.Itoa(port)
						pnode_id := trimPNodeID(port)
						send_data := psnode.TimeSync{
							PNodeID:     pnode_id,
							CurrentTime: time.Now(),
						}
						url := "http://localhost:" + port_str + "/time"
						client_data, err := json.Marshal(send_data)
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
					}(i)
				}
			}
			wg.Wait()
		}
	*/

	for k := 0; k < 10; k++ {
		var wg sync.WaitGroup
		for i := 21000; i < 21010; i++ {
			wg.Add(1)
			go func(port int) {
				defer wg.Done()
				port_str := strconv.Itoa(port)
				pnode_id := trimPNodeID(port)
				send_data := psnode.TimeSync{
					PNodeID:     pnode_id,
					CurrentTime: time.Now(),
				}
				url := "http://localhost:" + port_str + "/time"
				client_data, err := json.Marshal(send_data)
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
			}(i)
		}
		wg.Wait()
	}

	elapsedTime := time.Since(start_time)
	fmt.Println("execution time: ", elapsedTime)
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
	base_port, _ := strconv.Atoi(os.Getenv("PSNODE_BASE_PORT"))
	id_index := port - base_port
	pnode_id_int := int(0b0010<<60) + id_index
	pnode_id := strconv.Itoa(pnode_id_int)
	return pnode_id
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
