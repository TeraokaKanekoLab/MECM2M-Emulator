package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mecm2m-Emulator/pkg/message"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/joho/godotenv"
)

func main() {
	loadEnv()

	internet_link_process_exec_file := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/LinkProcess/Internet/main"
	fmt.Println(internet_link_process_exec_file)
	cmdInternet := exec.Command(internet_link_process_exec_file)
	errCmdInternet := cmdInternet.Run()
	if errCmdInternet != nil {
		message.MyError(errCmdInternet, "exec.Command > Internet > Start")
	} else {
		fmt.Println(internet_link_process_exec_file, " is running")
	}
	fmt.Println("Internet Link Process pid: ", cmdInternet.Process.Pid)

	/*
		cloud_server_path := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/CloudServer/Server/socket_files/server_0.json"
		cloud_server_exec_file := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/CloudServer/Server/main"
		cmdCloudServer := exec.Command(cloud_server_exec_file, cloud_server_path) // 2023-05-05 ソケットファイルの指定が必須 (フルパス)
		errCmdCloudServer := cmdCloudServer.Run()
		if errCmdCloudServer != nil {
			message.MyError(errCmdCloudServer, "exec.Command > Cloud Server > Start")
		} else {
			fmt.Println(cloud_server_exec_file, " is running")
		}
	*/
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
